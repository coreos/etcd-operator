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

	"github.com/coreos/etcd-operator/pkg/cluster"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"k8s.io/kubernetes/pkg/api"
)

func TestCreateCluster(t *testing.T) {
	f := framework.Global
	testEtcd, err := e2eutil.CreateEtcdCluster(f, e2eutil.MakeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
}

func TestResizeCluster3to5(t *testing.T) {
	f := framework.Global
	testEtcd, err := e2eutil.CreateEtcdCluster(f, e2eutil.MakeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
		return
	}
	fmt.Println("reached to 3 members cluster")

	testEtcd.Spec.Size = 5
	if err := e2eutil.UpdateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if _, err := e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 5, 60); err != nil {
		t.Fatalf("failed to resize to 5 members etcd cluster: %v", err)
	}
}

func TestResizeCluster5to3(t *testing.T) {
	f := framework.Global
	testEtcd, err := e2eutil.CreateEtcdCluster(f, e2eutil.MakeEtcdCluster("test-etcd-", 5))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 5, 90); err != nil {
		t.Fatalf("failed to create 5 members etcd cluster: %v", err)
		return
	}
	fmt.Println("reached to 5 members cluster")

	testEtcd.Spec.Size = 3
	if err := e2eutil.UpdateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if _, err := e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

func TestOneMemberRecovery(t *testing.T) {
	f := framework.Global
	testEtcd, err := e2eutil.CreateEtcdCluster(f, e2eutil.MakeEtcdCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()

	names, err := e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 3, 60)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
		return
	}
	fmt.Println("reached to 3 members cluster")

	if err := killMembers(f, names[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 3, 60); err != nil {
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

func testDisasterRecovery(t *testing.T, numToKill int) {
	f := framework.Global
	backupPolicy := &spec.BackupPolicy{
		SnapshotIntervalInSecond: 60 * 60,
		MaxSnapshot:              5,
		VolumeSizeInMB:           512,
		StorageType:              spec.BackupStorageTypePersistentVolume,
		CleanupOnClusterDelete:   true,
	}
	origEtcd := e2eutil.MakeEtcdCluster("test-etcd-", 3)
	origEtcd = e2eutil.EtcdClusterWithBackup(origEtcd, backupPolicy)
	testEtcd, err := e2eutil.CreateEtcdCluster(f, origEtcd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
		// TODO: add checking of removal of backup pod
	}()

	names, err := e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 3, 60)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")
	if err := e2eutil.WaitBackupPodUp(f, testEtcd.Name, 60*time.Second); err != nil {
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
	if _, err := e2eutil.WaitUntilSizeReached(f, testEtcd.Name, 3, 120); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
	// TODO: add checking of data in etcd
}

func makeBackup(f *framework.Framework, clusterName string) error {
	svc, err := f.KubeClient.Services(f.Namespace.Name).Get(k8sutil.MakeBackupName(clusterName))
	if err != nil {
		return err
	}
	// In our test environment, we assume kube-proxy should be running on the same node.
	// Thus we can use the service IP.
	ok := cluster.RequestBackupNow(f.KubeClient.Client, fmt.Sprintf("%s:%d", svc.Spec.ClusterIP, constants.DefaultBackupPodHTTPPort))
	if !ok {
		return fmt.Errorf("fail to request backupnow")
	}
	return nil
}

func killMembers(f *framework.Framework, names ...string) error {
	for _, name := range names {
		err := f.KubeClient.Pods(f.Namespace.Name).Delete(name, api.NewDeleteOptions(0))
		if err != nil {
			return err
		}
	}
	return nil
}
