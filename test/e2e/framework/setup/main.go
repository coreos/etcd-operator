package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/test/e2e/framework"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/util/wait"
)

const etcdTPRURL = "/apis/coreos.com/v1/etcdclusters"

func main() {
	kubeconfig := flag.String("kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
	ctrlImage := flag.String("controller-image", "", "controller image, e.g. gcr.io/coreos-k8s-scale-testing/kube-etcd-controller")
	flag.Parse()
	f, err := framework.New(*kubeconfig)
	if err != nil {
		panic(err)
	}
	if err := setupEtcdController(f, *ctrlImage); err != nil {
		panic(err)
	}
	logrus.Info("setup finished successfully")
}

func setupEtcdController(f *framework.Framework, ctrlImage string) error {
	// TODO: unify this and the yaml file in example/
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name:   "kube-etcd-controller",
			Labels: map[string]string{"name": "kube-etcd-controller"},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Name:  "kube-etcd-controller",
					Image: ctrlImage,
				},
			},
		},
	}
	_, err := f.KubeClient.Pods("default").Create(pod)
	if err != nil {
		return err
	}
	err = waitTPRReady(f)
	if err != nil {
		return err
	}
	logrus.Info("etcd controller created successfully")
	return nil
}

func waitTPRReady(f *framework.Framework) error {
	return wait.Poll(time.Second*20, time.Minute*5, func() (bool, error) {
		resp, err := f.KubeClient.Client.Get(f.MasterHost + etcdTPRURL)
		if err != nil {
			logrus.Errorf("http GET failed: %v", err)
			return false, err
		}
		switch resp.StatusCode {
		case http.StatusOK:
			return true, nil
		case http.StatusNotFound: // not set up yet. wait.
			logrus.Info("TPR not set up yet. Keep waiting...")
			return false, nil
		default:
			return false, fmt.Errorf("unexpected status code: %v", resp.Status)
		}
	})
}
