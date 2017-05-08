// Copyright 2017 The etcd-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package upgradetest

import (
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestResize(t *testing.T) {
	err := testF.CreateOperator()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := testF.DeleteOperator()
		if err != nil {
			t.Fatal(err)
		}
	}()
	if err = k8sutil.WaitEtcdTPRReady(testF.KubeCli.CoreV1().RESTClient(), 3*time.Second, 30*time.Second, testF.KubeNS); err != nil {
		t.Fatalf("failed to see cluster TPR get created in time: %v", err)
	}

	testClus, err := e2eutil.CreateCluster(t, testF.KubeCli, testF.KubeNS, e2eutil.NewCluster("upgrade-test-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteCluster(t, testF.KubeCli, testClus, &e2eutil.StorageCheckerOptions{}); err != nil {
			t.Fatal(err)
		}
	}()
	_, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 60*time.Second, testClus)
	if err != nil {
		t.Fatal(err)
	}
	err = testF.UpgradeOperator()
	if err != nil {
		t.Fatal(err)
	}

	testClus, err = k8sutil.GetClusterTPRObject(testF.KubeCli.CoreV1().RESTClient(), testF.KubeNS, testClus.Metadata.Name)
	if err != nil {
		t.Fatal(err)
	}

	updateFunc := func(cl *spec.Cluster) {
		cl.Spec.Size = 5
	}
	if _, err := e2eutil.UpdateCluster(testF.KubeCli, testClus, 10, updateFunc); err != nil {
		t.Fatal(err)
	}
	_, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 5, 60*time.Second, testClus)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHealOneMemberForOldCluster(t *testing.T) {
	err := testF.CreateOperator()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := testF.DeleteOperator()
		if err != nil {
			t.Fatal(err)
		}
	}()
	if err = k8sutil.WaitEtcdTPRReady(testF.KubeCli.CoreV1().RESTClient(), 3*time.Second, 30*time.Second, testF.KubeNS); err != nil {
		t.Fatalf("failed to see cluster TPR get created in time: %v", err)
	}

	testEtcd, err := e2eutil.CreateCluster(t, testF.KubeCli, testF.KubeNS, e2eutil.NewCluster("upgrade-test-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		checker := &e2eutil.StorageCheckerOptions{
			S3Cli:    testF.S3Cli,
			S3Bucket: testF.S3Bucket,
		}
		if err := e2eutil.DeleteCluster(t, testF.KubeCli, testEtcd, checker); err != nil {
			t.Fatal(err)
		}
	}()
	names, err := e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 60*time.Second, testEtcd)
	if err != nil {
		t.Fatal(err)
	}

	err = testF.UpgradeOperator()
	if err != nil {
		t.Fatal(err)
	}

	if err := e2eutil.KillMembers(testF.KubeCli, testF.KubeNS, names[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 60*time.Second, testEtcd); err != nil {
		t.Fatalf("failed to heal one member: %v", err)
	}
}

// TestBackupForOldCluster tests that new backup sidecar could make backup from old cluster.
func TestBackupForOldCluster(t *testing.T) {
	t.Run("PV backup for old cluster", func(t *testing.T) {
		testBackupForOldClusterWithBackupPolicy(t, e2eutil.NewPVBackupPolicy())
	})
	t.Run("S3 backup for old cluster", func(t *testing.T) {
		testBackupForOldClusterWithBackupPolicy(t, e2eutil.NewS3BackupPolicy())
	})
}

func testBackupForOldClusterWithBackupPolicy(t *testing.T, bp *spec.BackupPolicy) {
	err := testF.CreateOperator()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := testF.DeleteOperator()
		if err != nil {
			t.Fatal(err)
		}
	}()
	err = k8sutil.WaitEtcdTPRReady(testF.KubeCli.CoreV1().RESTClient(), 3*time.Second, 30*time.Second, testF.KubeNS)
	if err != nil {
		t.Fatalf("failed to see cluster TPR get created in time: %v", err)
	}

	bp.BackupIntervalInSecond = 60 * 60 * 24 // long enough that no backup was made automatically
	bp.CleanupBackupsOnClusterDelete = true
	cl := e2eutil.NewCluster("upgrade-test-", 3)
	cl = e2eutil.ClusterWithBackup(cl, bp)
	testClus, err := e2eutil.CreateCluster(t, testF.KubeCli, testF.KubeNS, cl)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		checker := &e2eutil.StorageCheckerOptions{
			S3Cli:    testF.S3Cli,
			S3Bucket: testF.S3Bucket,
		}
		if err := e2eutil.DeleteCluster(t, testF.KubeCli, testClus, checker); err != nil {
			t.Fatal(err)
		}
	}()
	_, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 60*time.Second, testClus)
	if err != nil {
		t.Fatal(err)
	}
	// "latest" image operator will create sidecar with versioned image. So we need to get sidecar image.
	oldBSImage, err := getContainerImageNameFromDeployment(k8sutil.BackupSidecarName(testClus.Metadata.Name))
	if err != nil {
		t.Fatal(err)
	}
	err = testF.UpgradeOperator()
	if err != nil {
		t.Fatal(err)
	}

	// We wait for old backup sidecar to be deleted so that we won't make backup from the old version.
	lo := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(k8sutil.BackupSidecarLabels(testClus.Metadata.Name)).String(),
	}
	_, err = e2eutil.WaitPodsWithImageDeleted(testF.KubeCli, testF.KubeNS, oldBSImage, 30*time.Second, lo)
	if err != nil {
		t.Fatal(err)
	}
	// Since old one is deleted, we can safely select any backup pod.
	err = e2eutil.WaitBackupPodUp(t, testF.KubeCli, testF.KubeNS, testClus.Metadata.Name, 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	err = e2eutil.MakeBackup(testF.KubeCli, testClus.Metadata.Namespace, testClus.Metadata.Name)
	if err != nil {
		t.Fatal(err)
	}
}

// get the image of the first container of the pod in the deployment.
func getContainerImageNameFromDeployment(name string) (string, error) {
	d, err := testF.KubeCli.AppsV1beta1().Deployments(testF.KubeNS).Get(name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return d.Spec.Template.Spec.Containers[0].Image, nil
}
