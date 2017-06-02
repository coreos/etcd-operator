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
		if err := e2eutil.DeleteCluster(t, testF.KubeCli, testClus); err != nil {
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
		if err := e2eutil.DeleteCluster(t, testF.KubeCli, testEtcd); err != nil {
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

// TestRestoreFromBackup tests that new operator could recover a new cluster from a backup of the old cluster.
func TestRestoreFromBackup(t *testing.T) {
	t.Run("Restore from PV backup of old cluster", func(t *testing.T) {
		testRestoreWithBackupPolicy(t, e2eutil.NewPVBackupPolicy(false))
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
	// create operator
	err := testF.CreateOperator()
	if err != nil {
		t.Fatalf("failed to create operator:%v", err)
	}
	defer func() {
		err := testF.DeleteOperator()
		if err != nil {
			t.Fatalf("failed to delete operator:%v", err)
		}
	}()
	err = k8sutil.WaitEtcdTPRReady(testF.KubeCli.CoreV1().RESTClient(), 3*time.Second, 30*time.Second, testF.KubeNS)
	if err != nil {
		t.Fatalf("failed to see cluster TPR get created in time: %v", err)
	}

	origClus := e2eutil.NewCluster("upgrade-test-", 3)
	origClus = e2eutil.ClusterWithBackup(origClus, bp)
	testClus, err := e2eutil.CreateCluster(t, testF.KubeCli, testF.KubeNS, origClus)
	if err != nil {
		t.Fatalf("failed to create cluster:%v", err)
	}
	names, err := e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 60*time.Second, testClus)
	if err != nil {
		t.Fatalf("failed to reach desired cluster size:%v", err)
	}
	err = e2eutil.WaitBackupPodUp(t, testF.KubeCli, testF.KubeNS, testClus.Metadata.Name, 60*time.Second)
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
	err = e2eutil.MakeBackup(testF.KubeCli, testClus.Metadata.Namespace, testClus.Metadata.Name)
	if err != nil {
		t.Fatalf("failed to make backup:%v", err)
	}

	// remove old cluster
	checker := e2eutil.StorageCheckerOptions{
		S3Cli:    testF.S3Cli,
		S3Bucket: testF.S3Bucket,
	}
	err = e2eutil.DeleteClusterAndBackup(t, testF.KubeCli, testClus, checker)
	if err != nil {
		t.Fatalf("failed to delete cluster and its backup:%v", err)
	}

	err = testF.UpgradeOperator()
	if err != nil {
		t.Fatal(err)
	}

	// create new cluster to restore from backup
	// Restore the etcd cluster of the same name:
	// - use the name already generated. We don't need to regenerate again.
	// - set BackupClusterName to the same name in RestorePolicy.
	// Then operator will use the existing backup in the same storage and
	// restore cluster with the same data.
	origClus.Metadata.GenerateName = ""
	origClus.Metadata.Name = testClus.Metadata.Name

	origClus = e2eutil.ClusterWithRestore(origClus, &spec.RestorePolicy{
		BackupClusterName: origClus.Metadata.Name,
		StorageType:       bp.StorageType,
	})
	origClus.Spec.Backup.CleanupBackupsOnClusterDelete = true
	testClus, err = e2eutil.CreateCluster(t, testF.KubeCli, testF.KubeNS, origClus)
	if err != nil {
		t.Fatalf("failed to create cluster:%v", err)
	}
	defer func() {
		checker := e2eutil.StorageCheckerOptions{
			S3Cli:    testF.S3Cli,
			S3Bucket: testF.S3Bucket,
		}
		err := e2eutil.DeleteClusterAndBackup(t, testF.KubeCli, testClus, checker)
		if err != nil {
			t.Fatalf("failed to delete cluster and its backup:%v", err)
		}
	}()

	names, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 240*time.Second, testClus)
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
		testBackupForOldClusterWithBackupPolicy(t, e2eutil.NewPVBackupPolicy(true))
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

	// Make interval long so no backup is made by the sidecar since we want to make only one backup during the whole test
	bp.BackupIntervalInSecond = 60 * 60 * 24
	cl := e2eutil.NewCluster("upgrade-test-", 3)
	cl = e2eutil.ClusterWithBackup(cl, bp)
	testClus, err := e2eutil.CreateCluster(t, testF.KubeCli, testF.KubeNS, cl)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		checker := e2eutil.StorageCheckerOptions{
			S3Cli:    testF.S3Cli,
			S3Bucket: testF.S3Bucket,
		}
		err := e2eutil.DeleteClusterAndBackup(t, testF.KubeCli, testClus, checker)
		if err != nil {
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

// TestDisasterRecovery tests if the new operator could do disaster recovery from backup of the old cluster.
func TestDisasterRecovery(t *testing.T) {
	t.Run("Recover from PV backup", func(t *testing.T) {
		testDisasterRecoveryWithBackupPolicy(t, e2eutil.NewPVBackupPolicy(true))
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
	err := testF.CreateOperator()
	if err != nil {
		t.Fatalf("failed to create operator: %v", err)
	}
	defer func() {
		err := testF.DeleteOperator()
		if err != nil {
			t.Fatalf("failed to delete operator: %v", err)
		}
	}()
	err = k8sutil.WaitEtcdTPRReady(testF.KubeCli.CoreV1().RESTClient(), 3*time.Second, 30*time.Second, testF.KubeNS)
	if err != nil {
		t.Fatalf("failed to see cluster TPR get created in time: %v", err)
	}

	testClus := e2eutil.NewCluster("upgrade-test-", 3)
	testClus = e2eutil.ClusterWithBackup(testClus, bp)
	testClus, err = e2eutil.CreateCluster(t, testF.KubeCli, testF.KubeNS, testClus)
	if err != nil {
		t.Fatalf("failed to create cluster: %v", err)
	}
	defer func() {
		checker := e2eutil.StorageCheckerOptions{
			S3Cli:    testF.S3Cli,
			S3Bucket: testF.S3Bucket,
		}
		err := e2eutil.DeleteClusterAndBackup(t, testF.KubeCli, testClus, checker)
		if err != nil {
			t.Fatalf("failed to delete cluster and its backup: %v", err)
		}
	}()

	names, err := e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 60*time.Second, testClus)
	if err != nil {
		t.Fatalf("failed to reach desired cluster size: %v", err)
	}
	err = e2eutil.WaitBackupPodUp(t, testF.KubeCli, testF.KubeNS, testClus.Metadata.Name, 60*time.Second)
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
	err = e2eutil.MakeBackup(testF.KubeCli, testClus.Metadata.Namespace, testClus.Metadata.Name)
	if err != nil {
		t.Fatalf("failed to make backup: %v", err)
	}

	err = testF.UpgradeOperator()
	if err != nil {
		t.Fatal(err)
	}

	if err := e2eutil.KillMembers(testF.KubeCli, testF.KubeNS, names...); err != nil {
		t.Fatalf("failed to kill all members: %v", err)
	}
	if names, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 180*time.Second, testClus); err != nil {
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
