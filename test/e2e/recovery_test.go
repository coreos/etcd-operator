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
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

func TestFailureRecovery(t *testing.T) {
	t.Run("failure recovery", func(t *testing.T) {
		t.Run("1 member (minority) down", testOneMemberRecovery)
		t.Run("2 members (majority) down", testDisasterRecovery2Members)
		t.Run("3 members (all) down", testDisasterRecoveryAll)
	})
}

func TestS3DisasterRecovery(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	t.Run("disaster recovery on S3", func(t *testing.T) {
		t.Run("2 members (majority) down", testS3MajorityDown)
		t.Run("3 members (all) down", testS3AllDown)
	})
}

func testOneMemberRecovery(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	f := framework.Global
	testEtcd, err := createCluster(t, f, newClusterSpec("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(t, f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	names, err := waitUntilSizeReached(t, f, testEtcd.Metadata.Name, 3, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")

	if err := killMembers(f, names[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := waitUntilSizeReached(t, f, testEtcd.Metadata.Name, 3, 60*time.Second); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

// testDisasterRecovery2Members tests disaster recovery that
// ooperator will make a backup from the left one pod.
func testDisasterRecovery2Members(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecovery(t, 2)
}

// testDisasterRecoveryAll tests disaster recovery that
// we should make a backup ahead and ooperator will recover cluster from it.
func testDisasterRecoveryAll(t *testing.T) {
	if os.Getenv(framework.EnvCloudProvider) == "aws" {
		t.Skip("skipping test due to relying on PodIP reachability. TODO: Remove this skip later")
	}
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecovery(t, 3)
}

func testDisasterRecovery(t *testing.T, numToKill int) {
	testDisasterRecoveryWithBackupPolicy(t, numToKill, newBackupPolicyPV())
}

func testDisasterRecoveryWithBackupPolicy(t *testing.T, numToKill int, backupPolicy *spec.BackupPolicy) {
	f := framework.Global

	backupPolicy.CleanupBackupsOnClusterDelete = true
	origEtcd := newClusterSpec("test-etcd-", 3)
	testEtcd, err := createCluster(t, f, etcdClusterWithBackup(origEtcd, backupPolicy))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(t, f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	names, err := waitUntilSizeReached(t, f, testEtcd.Metadata.Name, 3, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")
	if err := waitBackupPodUp(f, testEtcd.Metadata.Name, 60*time.Second); err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}
	// No left pod to make a backup from. We need to back up ahead.
	// If there is any left pod, ooperator should be able to make a backup from it.
	if numToKill == testEtcd.Spec.Size {
		if err := makeBackup(f, testEtcd.Metadata.Name); err != nil {
			t.Fatalf("fail to make a latest backup: %v", err)
		}
	} else {
		// Wait 2*5s to make sure the all pods are up and running.
		// Otherwise, the last member could have not come up yet.
		// Thus if we delete any member, it loses quorum and also exits.
		time.Sleep(10 * time.Second)
	}
	toKill := names[:numToKill]
	logfWithTimestamp(t, "killing pods: %v", toKill)
	// TODO: race: members could be recovered between being deleted one by one.
	if err := killMembers(f, toKill...); err != nil {
		t.Fatal(err)
	}
	if _, err := waitUntilSizeReached(t, f, testEtcd.Metadata.Name, 3, 120*time.Second); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
	// TODO: add checking of data in etcd
}

func testS3MajorityDown(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	testDisasterRecoveryWithBackupPolicy(t, 2, newBackupPolicyS3())
}

func testS3AllDown(t *testing.T) {
	if os.Getenv(framework.EnvCloudProvider) == "aws" {
		t.Skip("skipping test due to relying on PodIP reachability. TODO: Remove this skip later")
	}
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	testDisasterRecoveryWithBackupPolicy(t, 3, newBackupPolicyS3())
}
