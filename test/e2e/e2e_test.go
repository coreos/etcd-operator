package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/kube-etcd-controller/pkg/backup"
	"github.com/coreos/kube-etcd-controller/pkg/cluster"
	"github.com/coreos/kube-etcd-controller/pkg/util/constants"
	"github.com/coreos/kube-etcd-controller/test/e2e/framework"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/wait"
)

func TestCreateCluster(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3, nil))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
}

func TestResizeCluster3to5(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3, nil))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
		return
	}
	fmt.Println("reached to 3 members cluster")

	testEtcd.Spec.Size = 5
	if err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 5, 60); err != nil {
		t.Fatalf("failed to resize to 5 members etcd cluster: %v", err)
	}
}

func TestResizeCluster5to3(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 5, nil))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 5, 90); err != nil {
		t.Fatalf("failed to create 5 members etcd cluster: %v", err)
		return
	}
	fmt.Println("reached to 5 members cluster")

	testEtcd.Spec.Size = 3
	if err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

func TestOneMemberRecovery(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	names, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
		return
	}
	fmt.Println("reached to 3 members cluster")

	if err := killMembers(f, names[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

func TestDisasterRecovery(t *testing.T) {
	f := framework.Global
	backupPolicy := &backup.Policy{
		SnapshotIntervalInSecond: 120,
		MaxSnapshot:              5,
		VolumeSizeInMB:           512,
		StorageType:              backup.StorageTypePersistentVolume,
	}
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3, backupPolicy))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
		// TODO: add checking of removal of backup pod
	}()

	names, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
		return
	}
	fmt.Println("reached to 3 members cluster")
	if err := killMembers(f, names[0], names[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 120); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
	// TODO: add checking of data in etcd
}

func waitUntilSizeReached(f *framework.Framework, clusterName string, size int, timeout int) ([]string, error) {
	var names []string
	err := wait.Poll(5*time.Second, time.Duration(timeout)*time.Second, func() (done bool, err error) {
		pods, err := f.KubeClient.Pods(f.Namespace.Name).List(api.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"etcd_cluster": clusterName,
				"app":          "etcd",
			}),
		})
		if err != nil {
			logrus.Errorf("fail to list pods in cluster (%s): %v", clusterName, err)
			return false, err
		}
		names = getPodNames(pods.Items)
		logrus.Infof("Currently running pods: %v", names)
		if len(pods.Items) != size {
			// TODO: check etcd commands.
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	// check all members are ready
	for _, name := range names {
		cfg := clientv3.Config{
			Endpoints:   []string{fmt.Sprintf("http://%s:2379", name)},
			DialTimeout: constants.DefaultDialTimeout,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			logrus.Errorf("fail to create etcdcli (%s): %v", name, err)
			return nil, err
		}
	}
	return names, nil
}

func getPodNames(pods []api.Pod) []string {
	res := []string{}
	for _, p := range pods {
		res = append(res, p.Name)
	}
	return res
}

func killMembers(f *framework.Framework, names ...string) error {
	for _, name := range names {
		err := f.KubeClient.Pods(f.Namespace.Name).Delete(name, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func makeEtcdCluster(genName string, size int, backupPolicy *backup.Policy) *cluster.EtcdCluster {
	return &cluster.EtcdCluster{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "EtcdCluster",
			APIVersion: "coreos.com/v1",
		},
		ObjectMeta: api.ObjectMeta{
			GenerateName: genName,
		},
		Spec: cluster.Spec{
			Size:   size,
			Backup: backupPolicy,
		},
	}
}

func createEtcdCluster(f *framework.Framework, e *cluster.EtcdCluster) (*cluster.EtcdCluster, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	resp, err := f.KubeClient.Client.Post(
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters", f.MasterHost, f.Namespace.Name),
		"application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status: %v", resp.Status)
	}
	decoder := json.NewDecoder(resp.Body)
	res := &cluster.EtcdCluster{}
	if err := decoder.Decode(res); err != nil {
		return nil, err
	}
	return res, nil
}

func updateEtcdCluster(f *framework.Framework, e *cluster.EtcdCluster) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("PUT",
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters/%s", f.MasterHost, f.Namespace.Name, e.Name),
		bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.KubeClient.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %v", resp.Status)
	}
	return nil
}

func deleteEtcdCluster(f *framework.Framework, name string) error {
	// TODO: save etcd logs.
	req, err := http.NewRequest("DELETE",
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters/%s", f.MasterHost, f.Namespace.Name, name), nil)
	if err != nil {
		return err
	}
	resp, err := f.KubeClient.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %v", resp.Status)
	}
	return nil
}
