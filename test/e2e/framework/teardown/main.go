package main

import (
	"flag"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/test/e2e/framework"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
	flag.Parse()
	f, err := framework.New(*kubeconfig)
	if err != nil {
		panic(err)
	}
	// TODO: have unique test namespace (#117) and just delete everything under that.
	if err := teardownEtcdController(f); err != nil {
		panic(err)
	}
	logrus.Info("teardown finished successfully")
}

func teardownEtcdController(f *framework.Framework) error {
	err := f.KubeClient.Pods("default").Delete("kube-etcd-controller", nil)
	if err != nil {
		return err
	}
	logrus.Info("etcd controller deleted successfully")
	return nil
}
