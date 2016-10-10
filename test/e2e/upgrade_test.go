package e2e

import (
	"testing"

	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	"github.com/coreos/kube-etcd-controller/test/e2e/framework"
	"k8s.io/kubernetes/pkg/api"
)

func TestEtcdUpgrade(t *testing.T) {
	f := framework.Global
	origEtcd := makeEtcdCluster("test-etcd-", 3, nil)
	origEtcd = etcdClusterWithVersion(origEtcd, "v3.0.12")
	testEtcd, err := createEtcdCluster(f, origEtcd)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	_, err = waitSizeReachedWithFilter(f, testEtcd.Name, 3, 60, func(pod *api.Pod) bool {
		return k8sutil.GetEtcdVersion(pod) == "v3.0.12"
	})
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	testEtcd = etcdClusterWithVersion(testEtcd, "v3.1.0-alpha.1")

	if err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatalf("fail to update cluster version: %v", err)
	}

	_, err = waitSizeReachedWithFilter(f, testEtcd.Name, 3, 60, func(pod *api.Pod) bool {
		return k8sutil.GetEtcdVersion(pod) == "v3.1.0-alpha.1"
	})
	if err != nil {
		t.Fatalf("failed to wait new version etcd cluster: %v", err)
	}
}
