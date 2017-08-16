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
	"fmt"
	"math/rand"
	"testing"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func newOperatorName() string {
	suffix := fmt.Sprintf("-%d", rand.Uint64())
	return "etcd-operator" + suffix
}

func TestResize(t *testing.T) {
	name := newOperatorName()
	err := testF.CreateOperator(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := testF.DeleteOperator(name)
		if err != nil {
			t.Fatal(err)
		}
	}()
	err = e2eutil.WaitUntilOperatorReady(testF.KubeCli, testF.KubeNS, name)
	if err != nil {
		t.Fatal(err)
	}

	testClus, err := e2eutil.CreateCluster(t, testF.CRClient, testF.KubeNS, e2eutil.NewCluster("upgrade-test-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteCluster(t, testF.CRClient, testF.KubeCli, testClus); err != nil {
			t.Fatal(err)
		}
	}()
	_, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 6, testClus)
	if err != nil {
		t.Fatal(err)
	}
	err = testF.UpgradeOperator(name)
	if err != nil {
		t.Fatal(err)
	}

	testClus, err = k8sutil.GetClusterTPRObject(testF.KubeCli.CoreV1().RESTClient(), testF.KubeNS, testClus.Name)
	if err != nil {
		t.Fatal(err)
	}

	updateFunc := func(cl *spec.EtcdCluster) {
		cl.Spec.Size = 5
	}
	_, err = e2eutil.UpdateCluster(testF.CRClient, testClus, 10, updateFunc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 5, 6, testClus)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHealOneMemberForOldCluster(t *testing.T) {
	name := newOperatorName()
	err := testF.CreateOperator(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := testF.DeleteOperator(name)
		if err != nil {
			t.Fatal(err)
		}
	}()
	err = e2eutil.WaitUntilOperatorReady(testF.KubeCli, testF.KubeNS, name)
	if err != nil {
		t.Fatal(err)
	}

	testEtcd, err := e2eutil.CreateCluster(t, testF.CRClient, testF.KubeNS, e2eutil.NewCluster("upgrade-test-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteCluster(t, testF.CRClient, testF.KubeCli, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()
	names, err := e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 6, testEtcd)
	if err != nil {
		t.Fatal(err)
	}

	err = testF.UpgradeOperator(name)
	if err != nil {
		t.Fatal(err)
	}

	err = e2eutil.KillMembers(testF.KubeCli, testF.KubeNS, names[2])
	if err != nil {
		t.Fatal(err)
	}

	remaining, err := e2eutil.WaitUntilMembersWithNamesDeleted(t, testF.KubeCli, 3, testEtcd, names[2])
	if err != nil {
		t.Fatalf("failed to see members (%v) be deleted in time: %v", remaining, err)
	}

	_, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 6, testEtcd)
	if err != nil {
		t.Fatalf("failed to heal one member: %v", err)
	}
}

// TestRestoreFromBackup tests that new operator could recover a new cluster from a backup of the old cluster.
func TestRestoreFromBackup(t *testing.T) {
	t.Run("Restore from PV backup of old cluster", func(t *testing.T) {
		testRestoreWithBackupPolicy(t, e2eutil.NewPVBackupPolicy(false, ""))
	})
	t.Run("Restore from S3 backup of old cluster", func(t *testing.T) {
		t.Run("per cluster s3 policy", func(t *testing.T) {
			testRestoreWithBackupPolicy(t, e2eutil.NewS3BackupPolicy(false))
		})
		t.Run("operator wide s3 policy", func(t *testing.T) {
			testRestoreWithBackupPolicy(t, e2eutil.NewOperatorS3BackupPolicy(false))
		})
	})
}

func testRestoreWithBackupPolicy(t *testing.T, bp *spec.BackupPolicy) {
	name := newOperatorName()
	// create operator
	err := testF.CreateOperator(name)
	if err != nil {
		t.Fatalf("failed to create operator:%v", err)
	}
	defer func() {
		err := testF.DeleteOperator(name)
		if err != nil {
			t.Fatalf("failed to delete operator:%v", err)
		}
	}()
	err = e2eutil.WaitUntilOperatorReady(testF.KubeCli, testF.KubeNS, name)
	if err != nil {
		t.Fatal(err)
	}

	origClus := e2eutil.NewCluster("upgrade-test-", 3)
	origClus = e2eutil.ClusterWithBackup(origClus, bp)
	testClus, err := e2eutil.CreateCluster(t, testF.CRClient, testF.KubeNS, origClus)
	if err != nil {
		t.Fatalf("failed to create cluster:%v", err)
	}
	names, err := e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 6, testClus)
	if err != nil {
		t.Fatalf("failed to reach desired cluster size:%v", err)
	}
	err = e2eutil.WaitBackupPodUp(t, testF.KubeCli, testF.KubeNS, testClus.Name, 6)
	if err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}

	// write data to etcd and make a backup
	pod, err := testF.KubeCli.CoreV1().Pods(testF.KubeNS).Get(names[0], metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get backup pod:%v", err)
	}
	err = e2eutil.PutDataToEtcd(fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
	if err != nil {
		t.Fatalf("failed to put data to etcd:%v", err)
	}
	err = e2eutil.MakeBackup(testF.KubeCli, testClus.Namespace, testClus.Name)
	if err != nil {
		t.Fatalf("failed to make backup:%v", err)
	}

	// remove old cluster
	checker := e2eutil.StorageCheckerOptions{
		S3Cli:          testF.S3Cli,
		S3Bucket:       testF.S3Bucket,
		DeletedFromAPI: true,
	}
	err = e2eutil.DeleteClusterAndBackup(t, testF.CRClient, testF.KubeCli, testClus, checker)
	if err != nil {
		t.Fatalf("failed to delete cluster and its backup:%v", err)
	}

	err = testF.UpgradeOperator(name)
	if err != nil {
		t.Fatal(err)
	}

	// create new cluster to restore from backup
	// Restore the etcd cluster of the same name:
	// - use the name already generated. We don't need to regenerate again.
	// - set BackupClusterName to the same name in RestorePolicy.
	// Then operator will use the existing backup in the same storage and
	// restore cluster with the same data.
	origClus.GenerateName = ""
	origClus.Name = testClus.Name

	origClus = e2eutil.ClusterWithRestore(origClus, &spec.RestorePolicy{
		BackupClusterName: origClus.Name,
		StorageType:       bp.StorageType,
	})
	origClus.Spec.Backup.AutoDelete = true
	testClus, err = e2eutil.CreateCluster(t, testF.CRClient, testF.KubeNS, origClus)
	if err != nil {
		t.Fatalf("failed to create cluster:%v", err)
	}
	defer func() {
		checker.DeletedFromAPI = false
		err := e2eutil.DeleteClusterAndBackup(t, testF.CRClient, testF.KubeCli, testClus, checker)
		if err != nil {
			t.Fatalf("failed to delete cluster and its backup:%v", err)
		}
	}()

	names, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 24, testClus)
	if err != nil {
		t.Fatalf("failed to reach desired cluster size: %v", err)
	}

	// ensure the data from the previous cluster is present
	pod, err = testF.KubeCli.CoreV1().Pods(testF.KubeNS).Get(names[0], metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get backup pod:%v", err)
	}
	e2eutil.CheckEtcdData(t, fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
}

// TestBackupForOldCluster tests that new backup sidecar could make backup from old cluster.
func TestBackupForOldCluster(t *testing.T) {
	t.Run("PV backup for old cluster", func(t *testing.T) {
		testBackupForOldClusterWithBackupPolicy(t, e2eutil.NewPVBackupPolicy(true, ""))
	})
	t.Run("S3 backup for old cluster", func(t *testing.T) {
		t.Run("per cluster s3 policy", func(t *testing.T) {
			testBackupForOldClusterWithBackupPolicy(t, e2eutil.NewS3BackupPolicy(true))
		})
		t.Run("operator wide s3 policy", func(t *testing.T) {
			testBackupForOldClusterWithBackupPolicy(t, e2eutil.NewOperatorS3BackupPolicy(true))
		})
	})
}

func testBackupForOldClusterWithBackupPolicy(t *testing.T, bp *spec.BackupPolicy) {
	name := newOperatorName()
	err := testF.CreateOperator(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := testF.DeleteOperator(name)
		if err != nil {
			t.Fatal(err)
		}
	}()
	err = e2eutil.WaitUntilOperatorReady(testF.KubeCli, testF.KubeNS, name)
	if err != nil {
		t.Fatal(err)
	}

	// Make interval long so no backup is made by the sidecar since we want to make only one backup during the whole test
	bp.BackupIntervalInSecond = 60 * 60 * 24
	cl := e2eutil.NewCluster("upgrade-test-", 3)
	cl = e2eutil.ClusterWithBackup(cl, bp)
	testClus, err := e2eutil.CreateCluster(t, testF.CRClient, testF.KubeNS, cl)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		checker := e2eutil.StorageCheckerOptions{
			S3Cli:    testF.S3Cli,
			S3Bucket: testF.S3Bucket,
		}
		err := e2eutil.DeleteClusterAndBackup(t, testF.CRClient, testF.KubeCli, testClus, checker)
		if err != nil {
			t.Fatal(err)
		}
	}()
	_, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 6, testClus)
	if err != nil {
		t.Fatal(err)
	}
	// "latest" image operator will create sidecar with versioned image. So we need to get sidecar image.
	oldBSImage, err := getContainerImageNameFromDeployment(k8sutil.BackupSidecarName(testClus.Name))
	if err != nil {
		t.Fatal(err)
	}
	err = testF.UpgradeOperator(name)
	if err != nil {
		t.Fatal(err)
	}

	// We wait for old backup sidecar to be deleted so that we won't make backup from the old version.
	lo := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(k8sutil.BackupSidecarLabels(testClus.Name)).String(),
	}
	_, err = e2eutil.WaitPodsWithImageDeleted(testF.KubeCli, testF.KubeNS, oldBSImage, 6, lo)
	if err != nil {
		t.Fatal(err)
	}
	// Since old one is deleted, we can safely select any backup pod.
	err = e2eutil.WaitBackupPodUp(t, testF.KubeCli, testF.KubeNS, testClus.Name, 6)
	if err != nil {
		t.Fatal(err)
	}
	err = e2eutil.MakeBackup(testF.KubeCli, testClus.Namespace, testClus.Name)
	if err != nil {
		t.Fatal(err)
	}
}

// TestDisasterRecovery tests if the new operator could do disaster recovery from backup of the old cluster.
func TestDisasterRecovery(t *testing.T) {
	t.Run("Recover from PV backup", func(t *testing.T) {
		testDisasterRecoveryWithBackupPolicy(t, e2eutil.NewPVBackupPolicy(true, ""))
	})
	t.Run("Recover from S3 backup", func(t *testing.T) {
		t.Run("per cluster s3 policy", func(t *testing.T) {
			testDisasterRecoveryWithBackupPolicy(t, e2eutil.NewS3BackupPolicy(true))
		})
		t.Run("operator wide s3 policy", func(t *testing.T) {
			testDisasterRecoveryWithBackupPolicy(t, e2eutil.NewOperatorS3BackupPolicy(true))
		})
	})
}

func testDisasterRecoveryWithBackupPolicy(t *testing.T, bp *spec.BackupPolicy) {
	name := newOperatorName()
	err := testF.CreateOperator(name)
	if err != nil {
		t.Fatalf("failed to create operator: %v", err)
	}
	defer func() {
		err := testF.DeleteOperator(name)
		if err != nil {
			t.Fatalf("failed to delete operator: %v", err)
		}
	}()
	err = e2eutil.WaitUntilOperatorReady(testF.KubeCli, testF.KubeNS, name)
	if err != nil {
		t.Fatal(err)
	}

	testClus := e2eutil.NewCluster("upgrade-test-", 3)
	testClus = e2eutil.ClusterWithBackup(testClus, bp)
	testClus, err = e2eutil.CreateCluster(t, testF.CRClient, testF.KubeNS, testClus)
	if err != nil {
		t.Fatalf("failed to create cluster: %v", err)
	}
	defer func() {
		checker := e2eutil.StorageCheckerOptions{
			S3Cli:    testF.S3Cli,
			S3Bucket: testF.S3Bucket,
		}
		err := e2eutil.DeleteClusterAndBackup(t, testF.CRClient, testF.KubeCli, testClus, checker)
		if err != nil {
			t.Fatalf("failed to delete cluster and its backup: %v", err)
		}
	}()

	names, err := e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 6, testClus)
	if err != nil {
		t.Fatalf("failed to reach desired cluster size: %v", err)
	}
	err = e2eutil.WaitBackupPodUp(t, testF.KubeCli, testF.KubeNS, testClus.Name, 6)
	if err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}

	// write data to etcd and make a backup
	pod, err := testF.KubeCli.CoreV1().Pods(testF.KubeNS).Get(names[0], metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get backup pod: %v", err)
	}
	err = e2eutil.PutDataToEtcd(fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
	if err != nil {
		t.Fatalf("failed to put data to etcd: %v", err)
	}
	err = e2eutil.MakeBackup(testF.KubeCli, testClus.Namespace, testClus.Name)
	if err != nil {
		t.Fatalf("failed to make backup: %v", err)
	}

	err = testF.UpgradeOperator(name)
	if err != nil {
		t.Fatal(err)
	}

	err = e2eutil.KillMembers(testF.KubeCli, testF.KubeNS, names...)
	if err != nil {
		t.Fatalf("failed to kill all members: %v", err)
	}
	names, err = e2eutil.WaitUntilPodSizeReached(t, testF.KubeCli, 3, 18, testClus)
	if err != nil {
		t.Fatalf("failed to recover all members: %v", err)
	}

	// ensure the data from the previous cluster is present
	pod, err = testF.KubeCli.CoreV1().Pods(testF.KubeNS).Get(names[0], metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get backup pod: %v", err)
	}
	e2eutil.CheckEtcdData(t, fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
}

// get the image of the first container of the pod in the deployment.
func getContainerImageNameFromDeployment(name string) (string, error) {
	d, err := testF.KubeCli.AppsV1beta1().Deployments(testF.KubeNS).Get(name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return d.Spec.Template.Spec.Containers[0].Image, nil
}
