// Copyright 2016 The etcd-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coreos/etcd-operator/pkg/analytics"
	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/cluster"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	v1beta1extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

var (
	supportedPVProvisioners = map[string]struct{}{
		constants.PVProvisionerGCEPD:  {},
		constants.PVProvisionerAWSEBS: {},
		constants.PVProvisionerNone:   {},
	}

	ErrVersionOutdated = errors.New("requested version is outdated in apiserver")

	initRetryWaitTime = 30 * time.Second

	// Workaround for watching TPR resource.
	// client-go has encoding issue and we want something more predictable.
	KubeHttpCli *http.Client
	MasterHost  string
)

type Event struct {
	Type   kwatch.EventType
	Object *spec.Cluster
}

type Controller struct {
	logger *logrus.Entry
	Config

	// TODO: combine the three cluster map.
	clusters map[string]*cluster.Cluster
	// Kubernetes resource version of the clusters
	clusterRVs map[string]string
	stopChMap  map[string]chan struct{}

	waitCluster sync.WaitGroup
}

type Config struct {
	Namespace      string
	ServiceAccount string
	PVProvisioner  string
	s3config.S3Context
	KubeCli kubernetes.Interface
}

func (c *Config) Validate() error {
	if _, ok := supportedPVProvisioners[c.PVProvisioner]; !ok {
		return fmt.Errorf(
			"persistent volume provisioner %s is not supported: options = %v",
			c.PVProvisioner, supportedPVProvisioners,
		)
	}
	allEmpty := len(c.S3Context.AWSConfig) == 0 && len(c.S3Context.AWSSecret) == 0 && len(c.S3Context.S3Bucket) == 0
	allSet := len(c.S3Context.AWSConfig) != 0 && len(c.S3Context.AWSSecret) != 0 && len(c.S3Context.S3Bucket) != 0
	if !(allEmpty || allSet) {
		return errors.New("AWS/S3 related configs should be all set or all empty")
	}
	return nil
}

func New(cfg Config) *Controller {
	return &Controller{
		logger: logrus.WithField("pkg", "controller"),

		Config:     cfg,
		clusters:   make(map[string]*cluster.Cluster),
		clusterRVs: make(map[string]string),
		stopChMap:  map[string]chan struct{}{},
	}
}

func (c *Controller) Run() error {
	var (
		watchVersion string
		err          error
	)

	if len(c.Config.AWSConfig) != 0 {
		// AWS config/creds should be initialized only once here.
		// It will be shared and used by potential cluster's S3 backup manager to manage storage on operator side.
		err := setupS3Env(c.Config.KubeCli, c.Config.S3Context, c.Config.Namespace)
		if err != nil {
			return err
		}
	}

	for {
		watchVersion, err = c.initResource()
		if err == nil {
			break
		}
		c.logger.Errorf("initialization failed: %v", err)
		c.logger.Infof("retry in %v...", initRetryWaitTime)
		time.Sleep(initRetryWaitTime)
		// todo: add max retry?
	}

	c.logger.Infof("starts running from watch version: %s", watchVersion)

	defer func() {
		for _, stopC := range c.stopChMap {
			close(stopC)
		}
		c.waitCluster.Wait()
	}()

	eventCh, errCh := c.watch(watchVersion)

	go func() {
		pt := newPanicTimer(time.Minute, "unexpected long blocking (> 1 Minute) when handling cluster event")

		for ev := range eventCh {
			pt.start()
			if err := c.handleClusterEvent(ev); err != nil {
				c.logger.Warningf("fail to handle event: %v", err)
			}
			pt.stop()
		}
	}()
	return <-errCh
}

func (c *Controller) handleClusterEvent(event *Event) error {
	clus := event.Object

	if clus.Status.IsFailed() {
		if event.Type == kwatch.Deleted {
			delete(c.clusters, clus.Metadata.Name)
			delete(c.clusterRVs, clus.Metadata.Name)
			return nil
		}
		return fmt.Errorf("ignore failed cluster (%s). Please delete its TPR", clus.Metadata.Name)
	}

	clus.Spec.Cleanup()

	switch event.Type {
	case kwatch.Added:
		stopC := make(chan struct{})
		nc := cluster.New(c.makeClusterConfig(), clus, stopC, &c.waitCluster)

		c.stopChMap[clus.Metadata.Name] = stopC
		c.clusters[clus.Metadata.Name] = nc
		c.clusterRVs[clus.Metadata.Name] = clus.Metadata.ResourceVersion

		analytics.ClusterCreated()
		clustersCreated.Inc()
		clustersTotal.Inc()

	case kwatch.Modified:
		if _, ok := c.clusters[clus.Metadata.Name]; !ok {
			return fmt.Errorf("unsafe state. cluster was never created but we received event (%s)", event.Type)
		}
		c.clusters[clus.Metadata.Name].Update(clus)
		c.clusterRVs[clus.Metadata.Name] = clus.Metadata.ResourceVersion
		clustersModified.Inc()

	case kwatch.Deleted:
		if _, ok := c.clusters[clus.Metadata.Name]; !ok {
			return fmt.Errorf("unsafe state. cluster was never created but we received event (%s)", event.Type)
		}
		c.clusters[clus.Metadata.Name].Delete()
		delete(c.clusters, clus.Metadata.Name)
		delete(c.clusterRVs, clus.Metadata.Name)
		analytics.ClusterDeleted()
		clustersDeleted.Inc()
		clustersTotal.Dec()
	}
	return nil
}

func (c *Controller) findAllClusters() (string, error) {
	c.logger.Info("finding existing clusters...")
	clusterList, err := k8sutil.GetClusterList(c.Config.KubeCli.CoreV1().RESTClient(), c.Config.Namespace)
	if err != nil {
		return "", err
	}

	for i := range clusterList.Items {
		clus := clusterList.Items[i]

		if clus.Status.IsFailed() {
			c.logger.Infof("ignore failed cluster (%s). Please delete its TPR", clus.Metadata.Name)
			continue
		}

		clus.Spec.Cleanup()

		stopC := make(chan struct{})
		nc := cluster.New(c.makeClusterConfig(), &clus, stopC, &c.waitCluster)
		c.stopChMap[clus.Metadata.Name] = stopC
		c.clusters[clus.Metadata.Name] = nc
		c.clusterRVs[clus.Metadata.Name] = clus.Metadata.ResourceVersion
	}

	return clusterList.Metadata.ResourceVersion, nil
}

func (c *Controller) makeClusterConfig() cluster.Config {
	return cluster.Config{
		PVProvisioner:  c.PVProvisioner,
		ServiceAccount: c.Config.ServiceAccount,
		S3Context:      c.S3Context,

		KubeCli: c.KubeCli,
	}
}

func (c *Controller) initResource() (string, error) {
	watchVersion := "0"
	err := c.createTPR()
	if err != nil {
		if k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			// TPR has been initialized before. We need to recover existing cluster.
			watchVersion, err = c.findAllClusters()
			if err != nil {
				return "", err
			}
		} else {
			return "", fmt.Errorf("fail to create TPR: %v", err)
		}
	}
	if c.Config.PVProvisioner != constants.PVProvisionerNone {
		err = k8sutil.CreateStorageClass(c.KubeCli, c.PVProvisioner)
		if err != nil {
			if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
				return "", fmt.Errorf("fail to create storage class: %v", err)
			}
		}
	}
	return watchVersion, nil
}

func (c *Controller) createTPR() error {
	tpr := &v1beta1extensions.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: spec.TPRName(),
		},
		Versions: []v1beta1extensions.APIVersion{
			{Name: spec.TPRVersion},
		},
		Description: spec.TPRDescription,
	}
	_, err := c.KubeCli.ExtensionsV1beta1().ThirdPartyResources().Create(tpr)
	if err != nil {
		return err
	}

	return k8sutil.WaitEtcdTPRReady(c.KubeCli.CoreV1().RESTClient(), 3*time.Second, 30*time.Second, c.Namespace)
}

// watch creates a go routine, and watches the cluster.etcd kind resources from
// the given watch version. It emits events on the resources through the returned
// event chan. Errors will be reported through the returned error chan. The go routine
// exits on any error.
func (c *Controller) watch(watchVersion string) (<-chan *Event, <-chan error) {
	eventCh := make(chan *Event)
	// On unexpected error case, controller should exit
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)

		for {
			resp, err := k8sutil.WatchClusters(MasterHost, c.Config.Namespace, KubeHttpCli, watchVersion)
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				errCh <- errors.New("invalid status code: " + resp.Status)
				return
			}

			c.logger.Infof("start watching at %v", watchVersion)

			decoder := json.NewDecoder(resp.Body)
			for {
				ev, st, err := pollEvent(decoder)
				if err != nil {
					if err == io.EOF { // apiserver will close stream periodically
						c.logger.Debug("apiserver closed stream")
						break
					}

					c.logger.Errorf("received invalid event from API server: %v", err)
					errCh <- err
					return
				}

				if st != nil {
					resp.Body.Close()

					if st.Code == http.StatusGone {
						// event history is outdated.
						// if nothing has changed, we can go back to watch again.
						clusterList, err := k8sutil.GetClusterList(c.Config.KubeCli.CoreV1().RESTClient(), c.Config.Namespace)
						if err == nil && !c.isClustersCacheStale(clusterList.Items) {
							watchVersion = clusterList.Metadata.ResourceVersion
							break
						}

						// if anything has changed (or error on relist), we have to rebuild the state.
						// go to recovery path
						errCh <- ErrVersionOutdated
						return
					}

					c.logger.Fatalf("unexpected status response from API server: %v", st.Message)
				}

				c.logger.Debugf("etcd cluster event: %v %v", ev.Type, ev.Object.Spec)

				watchVersion = ev.Object.Metadata.ResourceVersion
				eventCh <- ev
			}

			resp.Body.Close()
		}
	}()

	return eventCh, errCh
}

func (c *Controller) isClustersCacheStale(currentClusters []spec.Cluster) bool {
	if len(c.clusterRVs) != len(currentClusters) {
		return true
	}

	for _, cc := range currentClusters {
		rv, ok := c.clusterRVs[cc.Metadata.Name]
		if !ok || rv != cc.Metadata.ResourceVersion {
			return true
		}
	}

	return false
}
