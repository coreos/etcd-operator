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
	"fmt"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/cluster"
	"github.com/coreos/etcd-operator/pkg/generated/clientset/versioned"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/sirupsen/logrus"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	kwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

var (
	supportedPVProvisioners = map[string]struct{}{
		constants.PVProvisionerGCEPD:  {},
		constants.PVProvisionerAWSEBS: {},
		constants.PVProvisionerNone:   {},
	}

	initRetryWaitTime = 30 * time.Second
)

type Event struct {
	Type   kwatch.EventType
	Object *api.EtcdCluster
}

type Controller struct {
	logger *logrus.Entry
	Config

	clusters map[string]*cluster.Cluster
}

type Config struct {
	Namespace      string
	ServiceAccount string
	PVProvisioner  string
	KubeCli        kubernetes.Interface
	KubeExtCli     apiextensionsclient.Interface
	EtcdCRCli      versioned.Interface
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

		Config:   cfg,
		clusters: make(map[string]*cluster.Cluster),
	}
}

func (c *Controller) handleClusterEvent(event *Event) error {
	clus := event.Object

	if clus.Status.IsFailed() {
		clustersFailed.Inc()
		if event.Type == kwatch.Deleted {
			delete(c.clusters, clus.Name)
			return nil
		}
		return fmt.Errorf("ignore failed cluster (%s). Please delete its CR", clus.Name)
	}

	clus.Spec.Cleanup()

	if err := clus.Spec.Validate(); err != nil {
		return fmt.Errorf("invalid cluster spec. please fix the following problem with the cluster spec: %v", err)
	}

	switch event.Type {
	case kwatch.Added:
		if _, ok := c.clusters[clus.Name]; ok {
			return fmt.Errorf("unsafe state. cluster (%s) was created before but we received event (%s)", clus.Name, event.Type)
		}

		nc := cluster.New(c.makeClusterConfig(), clus)

		c.clusters[clus.Name] = nc

		clustersCreated.Inc()
		clustersTotal.Inc()

	case kwatch.Modified:
		if _, ok := c.clusters[clus.Name]; !ok {
			return fmt.Errorf("unsafe state. cluster (%s) was never created but we received event (%s)", clus.Name, event.Type)
		}
		c.clusters[clus.Name].Update(clus)
		clustersModified.Inc()

	case kwatch.Deleted:
		if _, ok := c.clusters[clus.Name]; !ok {
			return fmt.Errorf("unsafe state. cluster (%s) was never created but we received event (%s)", clus.Name, event.Type)
		}
		c.clusters[clus.Name].Delete()
		delete(c.clusters, clus.Name)
		clustersDeleted.Inc()
		clustersTotal.Dec()
	}
	return nil
}

func (c *Controller) makeClusterConfig() cluster.Config {
	return cluster.Config{
		PVProvisioner:  c.PVProvisioner,
		ServiceAccount: c.Config.ServiceAccount,
		KubeCli:        c.Config.KubeCli,
		EtcdCRCli:      c.Config.EtcdCRCli,
	}
}

func (c *Controller) initCRD() error {
	err := k8sutil.CreateCRD(c.KubeExtCli)
	if err != nil {
		return err
	}
	return k8sutil.WaitCRDReady(c.KubeExtCli)
}
