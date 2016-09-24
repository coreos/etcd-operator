package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/pkg/cluster"
	"github.com/coreos/kube-etcd-controller/test/e2e/framework"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/wait"
)

func TestCreateCluster(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if err := waitUntilSizeReached(f, testEtcd.Name, 3); err != nil {
		t.Errorf("failed to create 3 members etcd cluster: %v", err)
	}
}

func TestResizeCluster3to5(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if err := waitUntilSizeReached(f, testEtcd.Name, 3); err != nil {
		t.Errorf("failed to create 3 members etcd cluster: %v", err)
	}
	t.Log("reached to 3 members cluster")

	testEtcd.Spec.Size = 5
	if err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if err := waitUntilSizeReached(f, testEtcd.Name, 5); err != nil {
		t.Errorf("failed to resize to 5 members etcd cluster: %v", err)
	}
}

func TestResizeCluster5to3(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 5))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if err := waitUntilSizeReached(f, testEtcd.Name, 5); err != nil {
		t.Errorf("failed to create 5 members etcd cluster: %v", err)
	}
	t.Log("reached to 5 members cluster")

	testEtcd.Spec.Size = 3
	if err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if err := waitUntilSizeReached(f, testEtcd.Name, 3); err != nil {
		t.Errorf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

func waitUntilSizeReached(f *framework.Framework, name string, size int) error {
	return wait.Poll(5*time.Second, 1*time.Minute, func() (done bool, err error) {
		pods, err := f.KubeClient.Pods(f.Namespace.Name).List(api.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"etcd_cluster": name,
			}),
		})
		if err != nil {
			return false, err
		}
		logrus.Infof("Currently running pods: %v", getPodNames(pods.Items))
		if len(pods.Items) != size {
			// TODO: check etcd commands.
			return false, nil
		}
		return true, nil
	})
}

func getPodNames(pods []api.Pod) []string {
	res := []string{}
	for _, p := range pods {
		res = append(res, p.Name)
	}
	return res
}

func makeEtcdCluster(genName string, size int) *cluster.EtcdCluster {
	return &cluster.EtcdCluster{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "EtcdCluster",
			APIVersion: "coreos.com/v1",
		},
		ObjectMeta: api.ObjectMeta{
			GenerateName: genName,
		},
		Spec: cluster.Spec{
			Size: size,
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
