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
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

func TestCreateCluster(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
}

func TestResizeCluster3to5(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")

	testEtcd.Spec.Size = 5
	if _, err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 5, 60); err != nil {
		t.Fatalf("failed to resize to 5 members etcd cluster: %v", err)
	}
}

func TestResizeCluster5to3(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 5))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 5, 90); err != nil {
		t.Fatalf("failed to create 5 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 5 members cluster")

	testEtcd.Spec.Size = 3
	if _, err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

func TestOneMemberRecovery(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	names, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")

	if err := killMembers(f, names[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

// TestDisasterRecovery2Members tests disaster recovery that
// ooperator will make a backup from the left one pod.
func TestDisasterRecovery2Members(t *testing.T) {
	testDisasterRecovery(t, 2)
}

// TestDisasterRecoveryAll tests disaster recovery that
// we should make a backup ahead and ooperator will recover cluster from it.
func TestDisasterRecoveryAll(t *testing.T) {
	testDisasterRecovery(t, 3)
}

// TestPauseControl tests the user can pause the operator from controlling
// an etcd cluster.
func TestPauseControl(t *testing.T) {
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	names, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	testEtcd.Spec.Paused = true
	if testEtcd, err = updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatalf("failed to pause control: %v", err)
	}

	// TODO: this is used to wait for the TPR to be updated.
	// TODO: make this wait for reliable
	time.Sleep(5 * time.Second)

	if err := killMembers(f, names[0]); err != nil {
		t.Fatal(err)
	}

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 2, 30); err != nil {
		t.Fatalf("failed to wait for killed member to die: %v", err)
	}
	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 30); err == nil {
		t.Fatalf("cluster should not be recovered: control is paused")
	}

	testEtcd.Spec.Paused = false
	if _, err = updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatalf("failed to resume control: %v", err)
	}

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

func testDisasterRecovery(t *testing.T, numToKill int) {
	f := framework.Global
	backupPolicy := &spec.BackupPolicy{
		SnapshotIntervalInSecond: 60 * 60,
		MaxSnapshot:              5,
		VolumeSizeInMB:           512,
		StorageType:              spec.BackupStorageTypePersistentVolume,
		CleanupOnClusterDelete:   true,
	}
	origEtcd := makeEtcdCluster("test-etcd-", 3)
	origEtcd = etcdClusterWithBackup(origEtcd, backupPolicy)
	testEtcd, err := createEtcdCluster(f, origEtcd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
		// TODO: add checking of removal of backup pod
	}()

	names, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60)
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
		toKill[i] = names[i]
	}
	// TODO: There might be race that ooperator will recover members between
	// 		these members are deleted individually.
	if err := killMembers(f, toKill...); err != nil {
		t.Fatal(err)
	}
	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 120); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
	// TODO: add checking of data in etcd
}
