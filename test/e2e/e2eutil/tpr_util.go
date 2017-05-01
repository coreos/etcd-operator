package e2eutil

import (
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/client-go/kubernetes"
)

func UpdateEtcdCluster(kubeClient kubernetes.Interface, cl *spec.Cluster, maxRetries int, updateFunc k8sutil.ClusterTPRUpdateFunc) (*spec.Cluster, error) {
	return k8sutil.AtomicUpdateClusterTPRObject(kubeClient.CoreV1().RESTClient(), cl.Metadata.Name, cl.Metadata.Namespace, maxRetries, updateFunc)
}
