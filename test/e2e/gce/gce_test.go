package gce

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

var fwConfig framework.Config

func init() {
	flag.StringVar(&fwConfig.KubeConfig, "kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
	flag.StringVar(&fwConfig.OpImage, "operator-image", "", "operator image, e.g. gcr.io/coreos-k8s-scale-testing/etcd-operator")
	flag.StringVar(&fwConfig.Namespace, "namespace", "default", "e2e test namespace")
	flag.Parse()
}

func TestNoPanicWithWrongPVProvisioner(t *testing.T) {
	fwConfig.PVProvisioner = "kubernetes.io/aws-ebs"
	// create operator with wrong pv provisioner
	if err := framework.Setup(fwConfig); err != nil {
		t.Fatal(err)
	}
	f := framework.Global
	defer func() {
		if err := framework.Teardown(); err != nil {
			t.Fatal(err)
		}
	}()
	origEtcd := e2eutil.MakeEtcdCluster("test-etcd-", 3)
	backupPolicy := e2eutil.MakeBackupPolicy()
	origEtcdWithBK := e2eutil.EtcdClusterWithBackup(*origEtcd, backupPolicy)

	// create a cluster with backup, wait and should see no etcd pods
	testEtcdWithBK, err := e2eutil.CreateEtcdCluster(f, origEtcdWithBK)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteEtcdCluster(f, testEtcdWithBK.Name); err != nil {
			t.Fatal(err)
		}
	}()

	fmt.Println("sleeping...")
	time.Sleep(20 * time.Second)
	fmt.Println("finished sleeping. Now checking etcd cluster.")

	podList, err := f.KubeClient.Pods(f.Namespace.Name).List(k8sutil.EtcdPodListOpt(testEtcdWithBK.Name))
	if err != nil {
		t.Fatal(err)
	}
	if len(podList.Items) != 0 {
		t.Fatalf("expect no etcd pods created, but have %d etcd pods", len(podList.Items))
	}

	fmt.Println("etcd cluster is dead as expected. Now creating a normal cluster...")
	// Create a cluster without backup, and the cluster should be run as normal.
	// This verifies that controller didn't crash
	testEtcd, err := e2eutil.CreateEtcdCluster(f, origEtcd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()
	_, err = e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 3, 60)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
}
