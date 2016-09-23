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
	myetcd := &cluster.EtcdCluster{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "EtcdCluster",
			APIVersion: "coreos.com/v1",
		},
		ObjectMeta: api.ObjectMeta{
			Name: "my-etcd",
		},
		Spec: cluster.Spec{
			Size: 3,
		},
	}
	body, err := json.Marshal(myetcd)
	if err != nil {
		t.Fatal(err)
	}

	if err := postEtcdCluster(f, body); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f); err != nil {
			t.Fatal(err)
		}
	}()

	err = wait.Poll(5*time.Second, 1*time.Minute, func() (done bool, err error) {
		pods, err := f.KubeClient.Pods(f.Namespace.Name).List(api.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"etcd_cluster": "my-etcd",
			}),
		})
		if err != nil {
			return false, err
		}
		logrus.Infof("Currently running pods: %v", getPodNames(pods.Items))
		if len(pods.Items) != 3 {
			// TODO: check etcd commands.
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Error("failed to create 3 members etcd cluster")
	}
}

func getPodNames(pods []api.Pod) []string {
	res := []string{}
	for _, p := range pods {
		res = append(res, p.Name)
	}
	return res
}

func postEtcdCluster(f *framework.Framework, body []byte) error {
	resp, err := f.KubeClient.Client.Post(
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters", f.MasterHost, f.Namespace.Name),
		"application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status: %v", resp.Status)
	}
	return nil
}

func deleteEtcdCluster(f *framework.Framework) error {
	req, err := http.NewRequest("DELETE",
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters/my-etcd", f.MasterHost, f.Namespace.Name), nil)
	if err != nil {
		return err
	}
	resp, err := f.KubeClient.Client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %v", resp.Status)
	}
	return nil
}
