// Copyright 2016 The etcd-operator Authors
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

package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPerClusS3RestoreSameName(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	testClusterRestoreS3SameName(t, true)
}

func TestOperWideS3RestoreSameName(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	testClusterRestoreS3SameName(t, false)
}

func TestPerClusS3RestoreDiffName(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	testClusterRestoreS3DifferentName(t, true)
}

func TestOperWideS3RestoreDiffName(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	testClusterRestoreS3DifferentName(t, false)
}

func TestClusterRestoreSameName(t *testing.T) {
	testClusterRestore(t, false)
}

func TestClusterRestoreDifferentName(t *testing.T) {
	testClusterRestore(t, true)
}

func testClusterRestore(t *testing.T, needDataClone bool) {
	testClusterRestoreWithBackupPolicy(t, needDataClone, e2eutil.NewPVBackupPolicy(false))
}

func testClusterRestoreWithBackupPolicy(t *testing.T, needDataClone bool, backupPolicy *spec.BackupPolicy) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	f := framework.Global

	origEtcd := e2eutil.NewCluster("test-etcd-", 3)
	testEtcd, err := e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, e2eutil.ClusterWithBackup(origEtcd, backupPolicy))
	if err != nil {
		t.Fatal(err)
	}

	names, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 60*time.Second, testEtcd)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	pod, err := f.KubeClient.CoreV1().Pods(f.Namespace).Get(names[0], metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	err = e2eutil.PutDataToEtcd(fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
	if err != nil {
		t.Fatal(err)
	}

	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, f.Namespace, testEtcd.Metadata.Name, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}
	err = e2eutil.MakeBackup(f.KubeClient, f.Namespace, testEtcd.Metadata.Name)
	if err != nil {
		t.Fatalf("fail to make a backup: %v", err)
	}

	var storageCheckerOptions *e2eutil.StorageCheckerOptions
	switch testEtcd.Spec.Backup.StorageType {
	case spec.BackupStorageTypePersistentVolume, spec.BackupStorageTypeDefault:
		storageCheckerOptions = &e2eutil.StorageCheckerOptions{}
	case spec.BackupStorageTypeS3:
		storageCheckerOptions = &e2eutil.StorageCheckerOptions{
			S3Cli:    f.S3Cli,
			S3Bucket: f.S3Bucket,
		}
	}
	err = e2eutil.DeleteClusterAndBackup(t, f.KubeClient, testEtcd, *storageCheckerOptions)
	if err != nil {
		t.Fatal(err)
	}
	// waits a bit to make sure resources are finally deleted on APIServer.
	time.Sleep(5 * time.Second)

	if !needDataClone {
		// Restore the etcd cluster of the same name:
		// - use the name already generated. We don't need to regenerate again.
		// - set BackupClusterName to the same name in RestorePolicy.
		// Then operator will use the existing backup in the same storage and
		// restore cluster with the same data.
		origEtcd.Metadata.GenerateName = ""
		origEtcd.Metadata.Name = testEtcd.Metadata.Name
	}
	waitRestoreTimeout := e2eutil.CalculateRestoreWaitTime(needDataClone)

	origEtcd = e2eutil.ClusterWithRestore(origEtcd, &spec.RestorePolicy{
		BackupClusterName: testEtcd.Metadata.Name,
		StorageType:       backupPolicy.StorageType,
	})

	backupPolicy.CleanupBackupsOnClusterDelete = true
	testEtcd, err = e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, e2eutil.ClusterWithBackup(origEtcd, backupPolicy))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		var storageCheckerOptions *e2eutil.StorageCheckerOptions
		switch testEtcd.Spec.Backup.StorageType {
		case spec.BackupStorageTypePersistentVolume, spec.BackupStorageTypeDefault:
			storageCheckerOptions = &e2eutil.StorageCheckerOptions{}
		case spec.BackupStorageTypeS3:
			storageCheckerOptions = &e2eutil.StorageCheckerOptions{
				S3Cli:    f.S3Cli,
				S3Bucket: f.S3Bucket,
			}
		}
		err := e2eutil.DeleteClusterAndBackup(t, f.KubeClient, testEtcd, *storageCheckerOptions)
		if err != nil {
			t.Fatal(err)
		}
	}()

	names, err = e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, waitRestoreTimeout, testEtcd)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	pod, err = f.KubeClient.CoreV1().Pods(f.Namespace).Get(names[0], metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e2eutil.CheckEtcdData(t, fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
}

func testClusterRestoreS3SameName(t *testing.T, perCluster bool) {
	var bp *spec.BackupPolicy
	if perCluster {
		bp = e2eutil.NewS3BackupPolicy(false)
	} else {
		bp = e2eutil.NewOperatorS3BackupPolicy(false)
	}

	testClusterRestoreWithBackupPolicy(t, false, bp)
}

func testClusterRestoreS3DifferentName(t *testing.T, perCluster bool) {
	var bp *spec.BackupPolicy
	if perCluster {
		bp = e2eutil.NewS3BackupPolicy(false)
	} else {
		bp = e2eutil.NewOperatorS3BackupPolicy(false)
	}

	testClusterRestoreWithBackupPolicy(t, true, bp)
}
