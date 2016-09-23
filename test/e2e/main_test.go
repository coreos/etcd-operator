package e2e

import (
	"flag"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/test/e2e/framework"
)

func TestMain(m *testing.M) {
	kubeconfig := flag.String("kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
	ctrlImage := flag.String("controller-image", "", "controller image, e.g. gcr.io/coreos-k8s-scale-testing/kube-etcd-controller")
	flag.Parse()

	f, err := framework.New(*kubeconfig, "top-level")
	if err != nil {
		logrus.Errorf("fail to create new framework: %v", err)
		os.Exit(1)
	}
	if err := f.Setup(*ctrlImage); err != nil {
		logrus.Errorf("fail to setup test environment: %v", err)
		os.Exit(1)
	}

	framework.Global = f
	code := m.Run()

	if err := f.Teardown(); err != nil {
		logrus.Errorf("fail to teardown test environment: %v", err)
		os.Exit(1)
	}
	os.Exit(code)
}
