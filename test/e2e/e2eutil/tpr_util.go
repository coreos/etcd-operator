package e2eutil

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/client-go/kubernetes"
)

func CreateCluster(t *testing.T, kubeClient kubernetes.Interface, namespace string, cl *spec.Cluster) (*spec.Cluster, error) {
	uri := fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters", spec.TPRGroup, spec.TPRVersion, namespace)
	b, err := kubeClient.CoreV1().RESTClient().Post().Body(cl).RequestURI(uri).DoRaw()
	if err != nil {
		return nil, err
	}
	res := &spec.Cluster{}
	if err := json.Unmarshal(b, res); err != nil {
		return nil, err
	}
	logfWithTimestamp(t, "created etcd cluster: %v", res.Metadata.Name)
	return res, nil
}

func UpdateEtcdCluster(kubeClient kubernetes.Interface, cl *spec.Cluster, maxRetries int, updateFunc k8sutil.ClusterTPRUpdateFunc) (*spec.Cluster, error) {
	return k8sutil.AtomicUpdateClusterTPRObject(kubeClient.CoreV1().RESTClient(), cl.Metadata.Name, cl.Metadata.Namespace, maxRetries, updateFunc)
}

func logfWithTimestamp(t *testing.T, format string, args ...interface{}) {
	t.Log(time.Now(), fmt.Sprintf(format, args...))
}
