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
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/pkg/analytics"
	"github.com/coreos/kube-etcd-controller/pkg/cluster"
	"github.com/coreos/kube-etcd-controller/pkg/spec"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

const (
	tprName = "etcd-cluster.coreos.com"
)

var (
	supportedPVProvisioners = map[string]struct{}{
		"kubernetes.io/gce-pd":  {},
		"kubernetes.io/aws-ebs": {},
	}
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
	clusters map[string]*cluster.Cluster
}

type Config struct {
	Namespace     string
	MasterHost    string
	KubeCli       *unversioned.Client
	PVProvisioner string
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
		Config:   cfg,
		clusters: make(map[string]*cluster.Cluster),
	}
}

func (c *Controller) Run() {
	watchVersion, err := c.initResource()
	if err != nil {
		panic(err)
	}
	log.Println("etcd cluster controller starts running...")

	eventCh, errCh := monitorEtcdCluster(c.MasterHost, c.Namespace, c.KubeCli.RESTClient.Client, watchVersion)
	for {
		select {
		case event := <-eventCh:
			clusterName := event.Object.ObjectMeta.Name
			switch event.Type {
			case "ADDED":
				clusterSpec := &event.Object.Spec
				nc := cluster.New(c.KubeCli, clusterName, c.Namespace, clusterSpec)
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
		case err := <-errCh:
			panic(err)
		}
	}
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
		nc := cluster.Restore(c.KubeCli, item.Name, c.Namespace, &item.Spec)
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

func monitorEtcdCluster(host, ns string, httpClient *http.Client, watchVersion string) (<-chan *Event, <-chan error) {
	eventCh := make(chan *Event)
	// On unexpected error case, controller should exit
	errCh := make(chan error, 1)
	go func() {
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
				ev, err := pollEvent(decoder)
				if err != nil {
					if err == io.EOF { // apiserver will close stream periodically
						break
					}
					errCh <- err
					return
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

func pollEvent(decoder *json.Decoder) (*Event, error) {
	re := &rawEvent{}
	err := decoder.Decode(re)
	if err != nil {
		if err == io.EOF {
			return nil, err
		}
		log.Errorf("fail to decode raw event from apiserver: %v", err)
		return nil, err
	}
	if re.Type == "ERROR" {
		log.Fatalf("watch error from apiserver. (TODO: separate transient and fatal error)\n"+
			"Error message: %s", re.Object)
	}
	ev := &Event{
		Type:   re.Type,
		Object: &spec.EtcdCluster{},
	}
	err = json.Unmarshal(re.Object, ev.Object)
	if err != nil {
		log.Errorf("fail to unmarshal EtcdCluster object from data (%s): %v", re.Object, err)
		return nil, err
	}
	return ev, nil
}
