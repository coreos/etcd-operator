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

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

func TestOneMemberRecovery(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	f := framework.Global
	testEtcd, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, e2eutil.NewCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, testEtcd)
		if err != nil {
			t.Fatal(err)
		}
	}()

	names, err := e2eutil.WaitUntilSizeReached(t, f.CRClient, 3, 6, testEtcd)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	t.Log("reached to 3 members cluster")

	// The last pod could have not come up serving yet. If we are not killing the last pod, we should wait.
	if err := e2eutil.KillMembers(f.KubeClient, f.Namespace, names[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := e2eutil.WaitUntilPodSizeReached(t, f.KubeClient, 3, 6, testEtcd); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

// TestDisasterRecoveryMaj ensures the operator will make a backup from the
// one remaining pod.
func TestDisasterRecoveryMaj(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecovery(t, 2, framework.Global.StorageClassName)
}

// testDisasterRecoveryAll tests disaster recovery that
// we should make a backup ahead and ooperator will recover cluster from it.
func TestDisasterRecoveryAll(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecovery(t, 3, framework.Global.StorageClassName)
}

func testDisasterRecovery(t *testing.T, numToKill int, storageClass string) {
	testDisasterRecoveryWithBackupPolicy(t, numToKill, e2eutil.NewPVBackupPolicy(true, storageClass))
}

func testDisasterRecoveryWithBackupPolicy(t *testing.T, numToKill int, backupPolicy *api.BackupPolicy) {
	cl := e2eutil.NewCluster("test-etcd-", 3)
	cl = e2eutil.ClusterWithBackup(cl, backupPolicy)
	testDisasterRecoveryWithCluster(t, numToKill, cl)
}

func testDisasterRecoveryWithCluster(t *testing.T, numToKill int, cl *api.EtcdCluster) {
	f := framework.Global

	testEtcd, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, cl)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		var storageCheckerOptions *e2eutil.StorageCheckerOptions
		switch testEtcd.Spec.Backup.StorageType {
		case api.BackupStorageTypePersistentVolume, api.BackupStorageTypeDefault:
			storageCheckerOptions = &e2eutil.StorageCheckerOptions{}
		case api.BackupStorageTypeS3:
			storageCheckerOptions = &e2eutil.StorageCheckerOptions{
				S3Cli:    f.S3Cli,
				S3Bucket: f.S3Bucket,
			}
		}

		err := e2eutil.DeleteClusterAndBackup(t, f.CRClient, f.KubeClient, testEtcd, *storageCheckerOptions)
		if err != nil {
			t.Fatal(err)
		}
	}()

	names, err := e2eutil.WaitUntilSizeReached(t, f.CRClient, 3, 6, testEtcd)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")
	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, f.Namespace, testEtcd.Name, 6)
	if err != nil {
		t.Fatalf("failed to wait backup pod up: %v", err)
	}
	// No left pod to make a backup from. We need to back up ahead.
	// If there is any left pod, ooperator should be able to make a backup from it.
	if numToKill == testEtcd.Spec.Size {
		err = e2eutil.MakeBackup(f.KubeClient, f.Namespace, testEtcd.Name)
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
	if _, err := e2eutil.WaitUntilPodSizeReached(t, f.KubeClient, 3, 12, testEtcd); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
	// TODO: add checking of data in etcd
}

func TestS3MajorityDown(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecoveryWithBackupPolicy(t, 2, e2eutil.NewS3BackupPolicy(true))
}

func TestS3AllDown(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecoveryWithBackupPolicy(t, 3, e2eutil.NewS3BackupPolicy(true))
}

func TestDynamicAddBackupPolicy(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	f := framework.Global
	clus, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, e2eutil.NewCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, clus)
		if err != nil {
			t.Fatal(err)
		}
	}()

	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, clus.Namespace, clus.Name, 6)
	if !retryutil.IsRetryFailure(err) {
		t.Fatalf("expecting retry failure, but err = %v", err)
	}

	uf := func(cl *api.EtcdCluster) {
		bp := e2eutil.NewS3BackupPolicy(true)
		cl.Spec.Backup = bp
	}
	clus, err = e2eutil.UpdateCluster(f.CRClient, clus, 10, uf)
	if err != nil {
		t.Fatal(err)
	}
	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, clus.Namespace, clus.Name, 6)
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
	clus, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, clus)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, clus)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = e2eutil.WaitUntilSizeReached(t, f.CRClient, 3, 6, clus)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, clus.Namespace, clus.Name, 6)
	if err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}

	uf := func(cl *api.EtcdCluster) {
		cl.Spec.Backup = nil
	}
	_, err = e2eutil.UpdateCluster(f.CRClient, clus, 10, uf)
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
