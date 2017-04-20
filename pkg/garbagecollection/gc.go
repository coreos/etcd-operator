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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	NullUID = ""
)

var pkgLogger = logrus.WithField("pkg", "gc")

type GC struct {
	logger *logrus.Entry

	kubecli kubernetes.Interface
	ns      string
}

func New(kubecli kubernetes.Interface, ns string) *GC {
	return &GC{
		logger:  pkgLogger,
		kubecli: kubecli,
		ns:      ns,
	}
}

// CollectCluster collects resources that matches cluster lable, but
// does not belong to the cluster with given clusterUID
func (gc *GC) CollectCluster(cluster string, clusterUID types.UID) {
	gc.collectResources(k8sutil.ClusterListOpt(cluster), map[types.UID]bool{clusterUID: true})
}

// FullyCollect collects resources that were created before,
// but does not belong to any current running clusters.
func (gc *GC) FullyCollect() error {
	clusters, err := k8sutil.GetClusterList(gc.kubecli.CoreV1().RESTClient(), gc.ns)
	if err != nil {
		return err
	}

	clusterUIDSet := make(map[types.UID]bool)
	for _, c := range clusters.Items {
		clusterUIDSet[c.Metadata.UID] = true
	}

	option := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app": "etcd",
		}).String(),
	}

	gc.collectResources(option, clusterUIDSet)
	return nil
}

func (gc *GC) collectResources(option metav1.ListOptions, runningSet map[types.UID]bool) {
	if err := gc.collectPods(option, runningSet); err != nil {
		gc.logger.Errorf("gc pods failed: %v", err)
	}
	if err := gc.collectServices(option, runningSet); err != nil {
		gc.logger.Errorf("gc services failed: %v", err)
	}
	if err := gc.collectReplicaSet(option, runningSet); err != nil {
		gc.logger.Errorf("gc replica set failed: %v", err)
	}
}

func (gc *GC) collectPods(option metav1.ListOptions, runningSet map[types.UID]bool) error {
	pods, err := gc.kubecli.CoreV1().Pods(gc.ns).List(option)
	if err != nil {
		return err
	}

	for _, p := range pods.Items {
		if len(p.OwnerReferences) == 0 {
			gc.logger.Warningf("failed to check pod %s: no owner", p.GetName())
			continue
		}
		// Pods failed due to liveness probe are also collected
		if !runningSet[p.OwnerReferences[0].UID] || p.Status.Phase == v1.PodFailed {
			// kill bad pods without grace period to kill it immediately
			err = gc.kubecli.CoreV1().Pods(gc.ns).Delete(p.GetName(), metav1.NewDeleteOptions(0))
			if err != nil && !k8sutil.IsKubernetesResourceNotFoundError(err) {
				return err
			}
			gc.logger.Infof("deleted pod (%v)", p.GetName())
		}
	}
	return nil
}

func (gc *GC) collectServices(option metav1.ListOptions, runningSet map[types.UID]bool) error {
	srvs, err := gc.kubecli.CoreV1().Services(gc.ns).List(option)
	if err != nil {
		return err
	}

	for _, srv := range srvs.Items {
		if len(srv.OwnerReferences) == 0 {
			gc.logger.Warningf("failed to check service %s: no owner", srv.GetName())
			continue
		}
		if !runningSet[srv.OwnerReferences[0].UID] {
			err = gc.kubecli.CoreV1().Services(gc.ns).Delete(srv.GetName(), nil)
			if err != nil && !k8sutil.IsKubernetesResourceNotFoundError(err) {
				return err
			}
			gc.logger.Infof("deleted service (%v)", srv.GetName())
		}
	}

	return nil
}

func (gc *GC) collectReplicaSet(option metav1.ListOptions, runningSet map[types.UID]bool) error {
	rss, err := gc.kubecli.ExtensionsV1beta1().ReplicaSets(gc.ns).List(option)
	if err != nil {
		return err
	}

	for _, rs := range rss.Items {
		if len(rs.OwnerReferences) == 0 {
			gc.logger.Warningf("failed to check replica set %s: no owner", rs.GetName())
			continue
		}
		if !runningSet[rs.OwnerReferences[0].UID] {
			// set orphanOption to false to enable Kubernetes GC to remove the objects that
			// depends on this replica set.
			// See https://kubernetes.io/docs/user-guide/garbage-collection/ for more details.
			orphanOption := false
			// set gracePeriod to delete the replica set immediately
			gracePeriod := int64(0)
			err = gc.kubecli.ExtensionsV1beta1().ReplicaSets(gc.ns).Delete(rs.GetName(), &metav1.DeleteOptions{
				OrphanDependents:   &orphanOption,
				GracePeriodSeconds: &gracePeriod,
			})
			if err != nil {
				if !k8sutil.IsKubernetesResourceNotFoundError(err) {
					return err
				}
			}
			gc.logger.Infof("deleted replica set (%s)", rs.GetName())
		}
	}

	return nil
}
