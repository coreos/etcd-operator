// Copyright 2017 The etcd-operator Authors
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

package garbagecollection

import (
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/labels"
	"k8s.io/client-go/1.5/pkg/types"
)

const (
	NullUID = ""
)

var pkgLogger = logrus.WithField("pkg", "gc")

type GC struct {
	logger *logrus.Entry

	k8s kubernetes.Interface
	ns  string
}

func New(k8s kubernetes.Interface, ns string) *GC {
	return &GC{
		logger: pkgLogger,
		k8s:    k8s,
		ns:     ns,
	}
}

// CollectCluster collects resources that matches cluster lable, but
// does not belong to the cluster with given clusterUID
func (gc *GC) CollectCluster(cluster string, clusterUID types.UID) error {
	return gc.collectResources(k8sutil.ClusterListOpt(cluster), map[types.UID]bool{clusterUID: true})
}

// FullyCollect collects resources that were created before,
// but does not belong to any current running clusters.
func (gc *GC) FullyCollect() error {
	clusters, err := k8sutil.GetClusterList(gc.k8s.Core().GetRESTClient(), gc.ns)
	if err != nil {
		return err
	}

	clusterUIDSet := make(map[types.UID]bool)
	for _, c := range clusters.Items {
		clusterUIDSet[c.GetUID()] = true
	}

	option := api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app": "etcd",
		}),
	}

	return gc.collectResources(option, clusterUIDSet)
}

func (gc *GC) collectResources(option api.ListOptions, runningSet map[types.UID]bool) error {
	if err := gc.collectPods(option, runningSet); err != nil {
		return err
	}
	if err := gc.collectServices(option, runningSet); err != nil {
		return err
	}
	if err := gc.collectReplicaSet(option, runningSet); err != nil {
		return err
	}

	return nil
}

func (gc *GC) collectPods(option api.ListOptions, runningSet map[types.UID]bool) error {
	pods, err := gc.k8s.Core().Pods(gc.ns).List(option)
	if err != nil {
		return err
	}

	for _, p := range pods.Items {
		if len(p.OwnerReferences) == 0 {
			gc.logger.Warningf("failed to check pod %s: no owner", p.GetName())
			continue
		}
		if !runningSet[p.OwnerReferences[0].UID] {
			// kill bad pods without grace period to kill it immediately
			err = gc.k8s.Core().Pods(gc.ns).Delete(p.GetName(), api.NewDeleteOptions(0))
			if err != nil {
				return err
			}
			gc.logger.Infof("deleted pod (%v)", p.GetName())
		}
	}
	return nil
}

func (gc *GC) collectServices(option api.ListOptions, runningSet map[types.UID]bool) error {
	srvs, err := gc.k8s.Core().Services(gc.ns).List(option)
	if err != nil {
		return err
	}

	for _, srv := range srvs.Items {
		if len(srv.OwnerReferences) == 0 {
			gc.logger.Warningf("failed to check service %s: no owner", srv.GetName())
			continue
		}
		if !runningSet[srv.OwnerReferences[0].UID] {
			err = gc.k8s.Core().Services(gc.ns).Delete(srv.GetName(), nil)
			if err != nil {
				return err
			}
			gc.logger.Infof("deleted service (%v)", srv.GetName())
		}
	}

	return nil
}

func (gc *GC) collectReplicaSet(option api.ListOptions, runningSet map[types.UID]bool) error {
	rss, err := gc.k8s.Extensions().ReplicaSets(gc.ns).List(option)
	if err != nil {
		return err
	}

	for _, rs := range rss.Items {
		if len(rs.OwnerReferences) == 0 {
			gc.logger.Warningf("failed to check replica set %s: no owner", rs.GetName())
			continue
		}
		if !runningSet[rs.OwnerReferences[0].UID] {
			err = gc.k8s.Extensions().ReplicaSets(gc.ns).Delete(rs.GetName(), nil)
			if err != nil {
				return err
			}
			gc.logger.Infof("deleted replica set (%s)", rs.GetName())
		}
	}

	return nil
}
