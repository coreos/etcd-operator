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

package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

func TestPerClusterS3MajDown(t *testing.T) {
	testS3MajorityDown(t, true)
}

func TestOperWideS3MajDown(t *testing.T) {
	testS3MajorityDown(t, true)
}

func TestPerClusterS3AllDown(t *testing.T) {
	testS3AllDown(t, true)
}

func TestOperWideS3AllDown(t *testing.T) {
	testS3AllDown(t, false)
}

func TestOneMemberRecovery(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	f := framework.Global
	testEtcd, err := e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, e2eutil.NewCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := e2eutil.DeleteCluster(t, f.KubeClient, testEtcd)
		if err != nil {
			t.Fatal(err)
		}
	}()

	names, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 60*time.Second, testEtcd)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	t.Log("reached to 3 members cluster")

	// The last pod could have not come up serving yet. If we are not killing the last pod, we should wait.
	if err := e2eutil.KillMembers(f.KubeClient, f.Namespace, names[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 60*time.Second, testEtcd); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

// TestDisasterRecoveryMaj tests disaster recovery that
// ooperator will make a backup from the left one pod.
func TestDisasterRecoveryMaj(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecovery(t, 2)
}

// testDisasterRecoveryAll tests disaster recovery that
// we should make a backup ahead and ooperator will recover cluster from it.
func TestDisasterRecoveryAll(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecovery(t, 3)
}

func testDisasterRecovery(t *testing.T, numToKill int) {
	testDisasterRecoveryWithBackupPolicy(t, numToKill, e2eutil.NewPVBackupPolicy(true))
}

func testDisasterRecoveryWithBackupPolicy(t *testing.T, numToKill int, backupPolicy *spec.BackupPolicy) {
	cl := e2eutil.NewCluster("test-etcd-", 3)
	cl = e2eutil.ClusterWithBackup(cl, backupPolicy)
	testDisasterRecoveryWithCluster(t, numToKill, cl)
}

func testDisasterRecoveryWithCluster(t *testing.T, numToKill int, cl *spec.Cluster) {
	f := framework.Global

	testEtcd, err := e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, cl)
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

	names, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 60*time.Second, testEtcd)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")
	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, f.Namespace, testEtcd.Metadata.Name, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}
	// No left pod to make a backup from. We need to back up ahead.
	// If there is any left pod, ooperator should be able to make a backup from it.
	if numToKill == testEtcd.Spec.Size {
		err = e2eutil.MakeBackup(f.KubeClient, f.Namespace, testEtcd.Metadata.Name)
		if err != nil {
			t.Fatalf("fail to make a latest backup: %v", err)
		}
	} else {
		// Wait 2*5s to make sure the all pods are up and running.
		// Otherwise, the last member could have not come up yet.
		// Thus if we delete any member, it loses quorum and also exits.
		time.Sleep(10 * time.Second)
	}
	toKill := names[:numToKill]
	e2eutil.LogfWithTimestamp(t, "killing pods: %v", toKill)
	// TODO: race: members could be recovered between being deleted one by one.
	if err := e2eutil.KillMembers(f.KubeClient, f.Namespace, toKill...); err != nil {
		t.Fatal(err)
	}
	if _, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 120*time.Second, testEtcd); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
	// TODO: add checking of data in etcd
}

func testS3MajorityDown(t *testing.T, perCluster bool) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	var bp *spec.BackupPolicy
	if perCluster {
		bp = e2eutil.NewS3BackupPolicy(true)
	} else {
		bp = e2eutil.NewOperatorS3BackupPolicy(true)
	}

	testDisasterRecoveryWithBackupPolicy(t, 2, bp)
}

func testS3AllDown(t *testing.T, perCluster bool) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	var bp *spec.BackupPolicy
	if perCluster {
		bp = e2eutil.NewS3BackupPolicy(true)
	} else {
		bp = e2eutil.NewOperatorS3BackupPolicy(true)
	}

	testDisasterRecoveryWithBackupPolicy(t, 3, bp)
}

func TestDynamicAddBackupPolicy(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	f := framework.Global
	clus, err := e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, e2eutil.NewCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := e2eutil.DeleteCluster(t, f.KubeClient, clus)
		if err != nil {
			t.Fatal(err)
		}
	}()

	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, clus.Metadata.Namespace, clus.Metadata.Name, 60*time.Second)
	if !retryutil.IsRetryFailure(err) {
		t.Fatalf("expecting retry failure, but err = %v", err)
	}

	uf := func(cl *spec.Cluster) {
		bp := e2eutil.NewS3BackupPolicy(true)
		cl.Spec.Backup = bp
	}
	clus, err = e2eutil.UpdateCluster(f.KubeClient, clus, 10, uf)
	if err != nil {
		t.Fatal(err)
	}
	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, clus.Metadata.Namespace, clus.Metadata.Name, 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDynamicRemoveBackupPolicy(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	f := framework.Global
	clus := e2eutil.ClusterWithBackup(e2eutil.NewCluster("test-etcd-", 3), e2eutil.NewS3BackupPolicy(true))
	clus, err := e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, clus)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := e2eutil.DeleteCluster(t, f.KubeClient, clus)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 60*time.Second, clus)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, clus.Metadata.Namespace, clus.Metadata.Name, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}

	uf := func(cl *spec.Cluster) {
		cl.Spec.Backup = nil
	}
	_, err = e2eutil.UpdateCluster(f.KubeClient, clus, 10, uf)
	if err != nil {
		t.Fatalf("failed to update cluster: %v", err)
	}

	storageCheckerOptions := e2eutil.StorageCheckerOptions{
		S3Cli:    f.S3Cli,
		S3Bucket: f.S3Bucket,
	}
	err = e2eutil.WaitBackupDeleted(f.KubeClient, clus, storageCheckerOptions)
	if err != nil {
		t.Fatalf("fail to wait backup deleted: %v", err)
	}
}
