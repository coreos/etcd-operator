// Copyright 2016 The kube-etcd-controller Authors
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

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/pkg/analytics"
	"github.com/coreos/kube-etcd-controller/pkg/cluster"
	"github.com/coreos/kube-etcd-controller/pkg/spec"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	k8sapi "k8s.io/kubernetes/pkg/api"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

const (
	tprName       = "etcd-cluster.coreos.com"
	statusTPRName = "etcd-cluster-status.coreos.com"
)

var (
	supportedPVProvisioners = map[string]struct{}{
		"kubernetes.io/gce-pd":  {},
		"kubernetes.io/aws-ebs": {},
	}

	ErrVersionOutdated = errors.New("requested version is outdated in apiserver")

	initRetryWaitTime = 30 * time.Second

	ReportStatusToTPR = false
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
	Config
	clusters    map[string]*cluster.Cluster
	stopChMap   map[string]chan struct{}
	waitCluster sync.WaitGroup
}

type Config struct {
	Namespace     string
	MasterHost    string
	KubeCli       *unversioned.Client
	PVProvisioner string
}

func init() {
	if ReportStatusToTPR {
		cluster.ReportStatusToTPR = true
	}
}

func (c *Config) validate() error {
	if _, ok := supportedPVProvisioners[c.PVProvisioner]; !ok {
		return fmt.Errorf(
			"persistent volume provisioner %s is not supported: options = %v",
			c.PVProvisioner, supportedPVProvisioners,
		)
	}
	return nil
}

func New(cfg Config) *Controller {
	if err := cfg.validate(); err != nil {
		panic(err)
	}
	return &Controller{
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
		log.Errorf("failed to initialize controller: %v", err)
		log.Infof("retry controller initialization in %v...", initRetryWaitTime)
		time.Sleep(initRetryWaitTime)
		// todo: add max retry?
	}

	log.Infof("etcd cluster controller starts running from watch version: %s", watchVersion)

	defer func() {
		for _, stopC := range c.stopChMap {
			close(stopC)
		}
		c.waitCluster.Wait()
	}()

	eventCh, errCh := monitorEtcdCluster(c.MasterHost, c.Namespace, c.KubeCli.RESTClient.Client, watchVersion)

	go func() {
		for event := range eventCh {
			clusterName := event.Object.ObjectMeta.Name
			switch event.Type {
			case "ADDED":
				clusterSpec := &event.Object.Spec
				stopC := make(chan struct{})
				c.stopChMap[clusterName] = stopC

				nc := cluster.New(c.KubeCli, c.MasterHost, clusterName, c.Namespace, clusterSpec, stopC, &c.waitCluster)
				c.clusters[clusterName] = nc
				analytics.ClusterCreated()

				backup := clusterSpec.Backup
				if backup != nil && backup.MaxSnapshot != 0 {
					err := k8sutil.CreateBackupReplicaSetAndService(c.KubeCli, clusterName, c.Namespace, *backup)
					if err != nil {
						panic(err)
					}
				}
			case "MODIFIED":
				c.clusters[clusterName].Update(&event.Object.Spec)
			case "DELETED":
				c.clusters[clusterName].Delete()
				delete(c.clusters, clusterName)
				analytics.ClusterDeleted()
			}
		}
	}()
	return <-errCh
}

func (c *Controller) findAllClusters() (string, error) {
	log.Println("finding existing clusters...")
	resp, err := k8sutil.ListETCDCluster(c.MasterHost, c.Namespace, c.KubeCli.RESTClient.Client)
	if err != nil {
		return "", err
	}
	d := json.NewDecoder(resp.Body)
	list := &EtcdClusterList{}
	if err := d.Decode(list); err != nil {
		return "", err
	}
	for _, item := range list.Items {
		stopC := make(chan struct{})
		c.stopChMap[item.Name] = stopC

		nc := cluster.Restore(c.KubeCli, c.MasterHost, item.Name, c.Namespace, &item.Spec, stopC, &c.waitCluster)
		c.clusters[item.Name] = nc

		backup := item.Spec.Backup
		if backup != nil && backup.MaxSnapshot != 0 {
			err := k8sutil.CreateBackupReplicaSetAndService(c.KubeCli, item.Name, c.Namespace, *backup)
			if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
				panic(err)
			}
		}
	}
	return list.ListMeta.ResourceVersion, nil
}

func (c *Controller) initResource() (string, error) {
	err := c.createTPR()
	if err != nil {
		switch {
		// etcd controller has been initialized before. We don't need to
		// repeat the init process but recover cluster.
		case k8sutil.IsKubernetesResourceAlreadyExistError(err):
			watchVersion, err := c.findAllClusters()
			if err != nil {
				return "", err
			}
			return watchVersion, nil
		default:
			log.Errorf("fail to create TPR: %v", err)
			return "", err
		}
	}

	err = c.createStatusTPR()
	if err != nil && !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
		log.Errorf("failed to create status TPR: %v", err)
		return "", err
	}

	err = k8sutil.CreateStorageClass(c.KubeCli, c.PVProvisioner)
	if err != nil {
		log.Errorf("fail to create storage class: %v", err)
		return "", err
	}
	return "0", nil
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

	return k8sutil.WaitEtcdTPRReady(c.KubeCli.Client, 3*time.Second, 30*time.Second, c.MasterHost, c.Namespace)
}

func (c *Controller) createStatusTPR() error {
	if !ReportStatusToTPR {
		return nil
	}

	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: k8sapi.ObjectMeta{
			Name: statusTPRName,
		},
		Versions: []extensions.APIVersion{
			{Name: "v1"},
		},
		Description: "Statuses of managed etcd clusters",
	}
	_, err := c.KubeCli.ThirdPartyResources().Create(tpr)
	if err != nil {
		return err
	}

	return nil
}

func monitorEtcdCluster(host, ns string, httpClient *http.Client, watchVersion string) (<-chan *Event, <-chan error) {
	eventCh := make(chan *Event)
	// On unexpected error case, controller should exit
	errCh := make(chan error, 1)
	go func() {
		defer close(eventCh)
		for {
			resp, err := k8sutil.WatchETCDCluster(host, ns, httpClient, watchVersion)
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				errCh <- errors.New("Invalid status code: " + resp.Status)
				return
			}
			log.Printf("watching at %v", watchVersion)
			decoder := json.NewDecoder(resp.Body)
			for {
				ev, st, err := pollEvent(decoder)
				if err != nil {
					if err == io.EOF { // apiserver will close stream periodically
						log.Debug("apiserver closed stream")
						break
					}
					errCh <- err
					return
				}
				if st != nil {
					if st.Code == http.StatusGone { // event history is outdated
						errCh <- ErrVersionOutdated // go to recovery path
						return
					}
					log.Fatalf("unexpected status response from apiserver: %v", st.Message)
				}
				log.Printf("etcd cluster event: %v %v", ev.Type, ev.Object.Spec)
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
		log.Errorf("fail to decode raw event from apiserver: %v", err)
		return nil, nil, err
	}
	if re.Type == "ERROR" {
		log.Errorf("watch error from apiserver: %s", re.Object)
		status := &unversionedAPI.Status{}
		err = json.Unmarshal(re.Object, status)
		if err != nil {
			log.Errorf("fail to decode (%s) into unversioned.Status: %v", re.Object, err)
			return nil, nil, err
		}
		return nil, status, nil
	}
	ev := &Event{
		Type:   re.Type,
		Object: &spec.EtcdCluster{},
	}
	err = json.Unmarshal(re.Object, ev.Object)
	if err != nil {
		log.Errorf("fail to unmarshal EtcdCluster object from data (%s): %v", re.Object, err)
		return nil, nil, err
	}
	return ev, nil, nil
}
