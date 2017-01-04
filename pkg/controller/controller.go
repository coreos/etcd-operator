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
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	k8sapi "k8s.io/kubernetes/pkg/api"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

const (
	tprName = "etcd-cluster.coreos.com"

	defaultVersion = "v3.1.0-alpha.1"
)

var (
	supportedPVProvisioners = map[string]struct{}{
		"kubernetes.io/gce-pd":  {},
		"kubernetes.io/aws-ebs": {},
	}

	ErrVersionOutdated = errors.New("requested version is outdated in apiserver")

	initRetryWaitTime = 30 * time.Second
)

type rawEvent struct {
	Type   string
	Object json.RawMessage
}

type Event struct {
	Type   string
	Object *spec.EtcdCluster
}

type Controller struct {
	logger *logrus.Entry

	Config
	clusters    map[string]*cluster.Cluster
	stopChMap   map[string]chan struct{}
	waitCluster sync.WaitGroup
}

type Config struct {
	MasterHost    string
	Namespace     string
	PVProvisioner string
	s3config.S3Context
	KubeCli *unversioned.Client
}

func (c *Config) Validate() error {
	if _, ok := supportedPVProvisioners[c.PVProvisioner]; !ok {
		return fmt.Errorf(
			"persistent volume provisioner %s is not supported: options = %v",
			c.PVProvisioner, supportedPVProvisioners,
		)
	}
	return nil
}

func New(cfg Config) *Controller {
	return &Controller{
		logger: logrus.WithField("pkg", "controller"),

		Config:    cfg,
		clusters:  make(map[string]*cluster.Cluster),
		stopChMap: map[string]chan struct{}{},
	}
}

func (c *Controller) Run() error {
	var (
		watchVersion string
		err          error
	)

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

	eventCh, errCh := c.monitor(watchVersion)

	go func() {
		for event := range eventCh {
			if s := event.Object.Spec; len(s.Version) == 0 {
				// TODO: set version in spec in apiserver
				s.Version = defaultVersion
			}

			clusterName := event.Object.ObjectMeta.Name
			switch event.Type {
			case "ADDED":
				stopC := make(chan struct{})
				nc := cluster.New(c.makeClusterConfig(), event.Object, stopC, &c.waitCluster)
				if nc == nil {
					continue
				}

				c.stopChMap[clusterName] = stopC
				c.clusters[clusterName] = nc

				analytics.ClusterCreated()
				clustersCreated.Inc()
				clustersTotal.Inc()

			case "MODIFIED":
				if c.clusters[clusterName] == nil {
					c.logger.Warningf("ignore modification: cluster %q not found (or dead)", clusterName)
					break
				}

				c.clusters[clusterName].Update(event.Object)
				clustersModified.Inc()

			case "DELETED":
				if c.clusters[clusterName] == nil {
					c.logger.Warningf("ignore deletion: cluster %q not found (or dead)", clusterName)
					break
				}

				c.clusters[clusterName].Delete()
				delete(c.clusters, clusterName)
				analytics.ClusterDeleted()
				clustersDeleted.Inc()
				clustersTotal.Dec()
			}
		}
	}()
	return <-errCh
}

func (c *Controller) findAllClusters() (string, error) {
	c.logger.Info("finding existing clusters...")
	clusterList, err := k8sutil.GetClusterList(c.Config.KubeCli, c.Config.MasterHost, c.Config.Namespace)
	if err != nil {
		return "", err
	}

	for _, item := range clusterList.Items {
		if s := item.Spec; len(s.Version) == 0 {
			// TODO: set version in spec in apiserver
			s.Version = defaultVersion
		}
		clusterName := item.Name
		stopC := make(chan struct{})
		nc := cluster.New(c.makeClusterConfig(), &item, stopC, &c.waitCluster)
		if nc == nil {
			continue
		}
		c.stopChMap[clusterName] = stopC
		c.clusters[clusterName] = nc
	}

	return clusterList.ResourceVersion, nil
}

func (c *Controller) makeClusterConfig() cluster.Config {
	return cluster.Config{
		PVProvisioner: c.PVProvisioner,
		S3Context:     c.S3Context,

		MasterHost: c.MasterHost,
		KubeCli:    c.KubeCli,
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
	err = k8sutil.CreateStorageClass(c.KubeCli, c.PVProvisioner)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return "", fmt.Errorf("fail to create storage class: %v", err)
		}
	}
	return watchVersion, nil
}

func (c *Controller) createTPR() error {
	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: k8sapi.ObjectMeta{
			Name: tprName,
		},
		Versions: []extensions.APIVersion{
			{Name: "v1"},
		},
		Description: "Managed etcd clusters",
	}
	_, err := c.KubeCli.ThirdPartyResources().Create(tpr)
	if err != nil {
		return err
	}

	return k8sutil.WaitEtcdTPRReady(c.KubeCli, 3*time.Second, 30*time.Second, c.MasterHost, c.Namespace)
}

func (c *Controller) monitor(watchVersion string) (<-chan *Event, <-chan error) {
	host := c.MasterHost
	ns := c.Namespace
	httpClient := c.KubeCli.Client

	eventCh := make(chan *Event)
	// On unexpected error case, controller should exit
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)

		for {
			resp, err := k8sutil.WatchClusters(host, ns, httpClient, watchVersion)
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				errCh <- errors.New("Invalid status code: " + resp.Status)
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
					if st.Code == http.StatusGone { // event history is outdated
						errCh <- ErrVersionOutdated // go to recovery path
						return
					}
					c.logger.Fatalf("unexpected status response from API server: %v", st.Message)
				}

				c.logger.Debugf("etcd cluster event: %v %v", ev.Type, ev.Object.Spec)

				watchVersion = ev.Object.ObjectMeta.ResourceVersion
				eventCh <- ev
			}

			resp.Body.Close()
		}
	}()

	return eventCh, errCh
}

func pollEvent(decoder *json.Decoder) (*Event, *unversionedAPI.Status, error) {
	re := &rawEvent{}
	err := decoder.Decode(re)
	if err != nil {
		if err == io.EOF {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("fail to decode raw event from apiserver (%v)", err)
	}

	if re.Type == "ERROR" {
		status := &unversionedAPI.Status{}
		err = json.Unmarshal(re.Object, status)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode (%s) into unversioned.Status (%v)", re.Object, err)
		}
		return nil, status, nil
	}

	ev := &Event{
		Type:   re.Type,
		Object: &spec.EtcdCluster{},
	}
	err = json.Unmarshal(re.Object, ev.Object)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to unmarshal EtcdCluster object from data (%s): %v", re.Object, err)
	}
	return ev, nil, nil
}
