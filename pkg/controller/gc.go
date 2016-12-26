package controller

import (
	"github.com/Sirupsen/logrus"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/types"
)

type gc struct {
	// reference to parent controller
	controller *Controller

	logger *logrus.Entry

	k8s *unversioned.Client
	ns  string
}

// collectCluster collects resources that matches cluster lable, but
// does not belong to the cluster with given clusterUID
func (gc *gc) collectCluster(cluster string, clusterUID types.UID) error {
	option := k8sapi.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": cluster,
			"app":          "etcd",
		}),
	}

	return gc.collectResources(option, map[types.UID]bool{clusterUID: true})
}

// fullyCollect collects resources that are created by the controller,
// but does not belong to any running etcd clusters.
func (gc *gc) fullyCollect() error {
	clusters, _, err := gc.controller.getClustersFromTPR()
	if err != nil {
		return err
	}

	clusterUIDSet := make(map[types.UID]bool)
	for _, c := range clusters {
		clusterUIDSet[c.GetUID()] = true
	}

	option := k8sapi.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app": "etcd",
		}),
	}

	return gc.collectResources(option, clusterUIDSet)
}

func (gc *gc) collectResources(option k8sapi.ListOptions, runningSet map[types.UID]bool) error {
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

func (gc *gc) collectPods(option k8sapi.ListOptions, runningSet map[types.UID]bool) error {
	pods, err := gc.k8s.Pods(gc.ns).List(option)
	if err != nil {
		return err
	}

	for _, p := range pods.Items {
		if len(p.OwnerReferences) == 0 {
			gc.logger.Warningf("failed to check pod %s", p.GetName())
			continue
		}
		if !runningSet[p.OwnerReferences[0].UID] {
			err = gc.k8s.Pods(gc.ns).Delete(p.GetName(), k8sapi.NewDeleteOptions(0))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (gc *gc) collectServices(option k8sapi.ListOptions, runningSet map[types.UID]bool) error {
	srvs, err := gc.k8s.Services(gc.ns).List(option)
	if err != nil {
		return err
	}

	for _, srv := range srvs.Items {
		if len(srv.OwnerReferences) == 0 {
			gc.logger.Warningf("failed to check srv %s", srv.GetName())
			continue
		}
		if !runningSet[srv.OwnerReferences[0].UID] {
			err = gc.k8s.Services(gc.ns).Delete(srv.GetName())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (gc *gc) collectReplicaSet(option k8sapi.ListOptions, runningSet map[types.UID]bool) error {
	rss, err := gc.k8s.ReplicaSets(gc.ns).List(option)
	if err != nil {
		return err
	}

	for _, rs := range rss.Items {
		if len(rs.OwnerReferences) == 0 {
			gc.logger.Warningf("failed to check replica set %s", rs.GetName())
			continue
		}
		if !runningSet[rs.OwnerReferences[0].UID] {
			err = gc.k8s.ReplicaSets(gc.ns).Delete(rs.GetName(), k8sapi.NewDeleteOptions(0))
			if err != nil {
				return err
			}
		}
	}

	return nil
}
