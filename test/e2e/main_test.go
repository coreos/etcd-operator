package e2e

import (
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/test/e2e/framework"
)

func TestMain(m *testing.M) {
	if err := framework.Setup(); err != nil {
		logrus.Errorf("fail to setup framework: %v", err)
		os.Exit(1)
	}

	code := m.Run()

	if err := framework.Teardown(); err != nil {
		logrus.Errorf("fail to teardown framework: %v", err)
		os.Exit(1)
	}
	os.Exit(code)
}
