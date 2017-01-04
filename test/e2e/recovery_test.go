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
		if os.Getenv(envParallelTest) == envParallelTestTrue {
			t.Parallel()
		}
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
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	names, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")

	if err := killMembers(f, names[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60*time.Second); err != nil {
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
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecovery(t, 3)
}

func testDisasterRecovery(t *testing.T, numToKill int) {
	testDisasterRecoveryWithStorageType(t, numToKill, spec.BackupStorageTypePersistentVolume)
}

func testDisasterRecoveryWithStorageType(t *testing.T, numToKill int, bt spec.BackupStorageType) {
	f := framework.Global
	origEtcd := makeEtcdCluster("test-etcd-", 3)
	bp := backupPolicyWithStorageType(makeBackupPolicy(true), bt)
	testEtcd, err := createEtcdCluster(f, etcdClusterWithBackup(origEtcd, bp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	names, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")
	if err := waitBackupPodUp(f, testEtcd.Name, 60*time.Second); err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}
	// No left pod to make a backup from. We need to back up ahead.
	// If there is any left pod, ooperator should be able to make a backup from it.
	if numToKill == len(names) {
		if err := makeBackup(f, testEtcd.Name); err != nil {
			t.Fatalf("fail to make a latest backup: %v", err)
		}
	}
	toKill := make([]string, numToKill)
	for i := 0; i < numToKill; i++ {
		toKill[i] = names[len(names)-i-1]
	}
	// TODO: There might be race that ooperator will recover members between
	// 		these members are deleted individually.
	if err := killMembers(f, toKill...); err != nil {
		t.Fatal(err)
	}
	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 120*time.Second); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
	// TODO: add checking of data in etcd
}

func testS3MajorityDown(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecoveryWithStorageType(t, 2, spec.BackupStorageTypeS3)
}

func testS3AllDown(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testDisasterRecoveryWithStorageType(t, 3, spec.BackupStorageTypeS3)
}
