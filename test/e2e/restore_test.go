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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

func TestClusterRestoreSameName(t *testing.T) {
	testClusterRestore(t, true)
}

func TestClusterRestoreDifferentName(t *testing.T) {
	testClusterRestore(t, false)
}

func testClusterRestore(t *testing.T, sameName bool) {
	f := framework.Global
	origEtcd := makeEtcdCluster("test-etcd-", 3)
	testEtcd, err := createEtcdCluster(f, etcdClusterWithBackup(origEtcd, makeBackupPolicy(false)))
	if err != nil {
		t.Fatal(err)
	}
	names, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	pod, err := f.KubeClient.Pods(f.Namespace.Name).Get(names[0])
	if err != nil {
		t.Fatal(err)
	}
	etcdcli, err := createEtcdClient(fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	_, err = etcdcli.Put(ctx, etcdKeyFoo, etcdValBar)
	cancel()
	etcdcli.Close()
	if err != nil {
		t.Fatal(err)
	}

	if err := waitBackupPodUp(f, testEtcd.Name, 60*time.Second); err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}
	if err := makeBackup(f, testEtcd.Name); err != nil {
		t.Fatalf("fail to make a backup: %v", err)
	}
	if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
		t.Fatal(err)
	}
	// waits a bit to make sure resources are finally deleted on APIServer.
	time.Sleep(5 * time.Second)

	if sameName {
		// Restore the etcd cluster of the same name:
		// - use the name already generated. We don't need to regenerate again.
		// - set BackupClusterName to the same name in RestorePolicy.
		// Then operator will use the existing backup in previous PVC and
		// restore cluster with the same data.
		origEtcd.GenerateName = ""
		origEtcd.Name = testEtcd.Name
	}
	// It could take very long due to delay of k8s controller detaching the volume
	waitRestoreTimeout := 180
	if !sameName {
		// even longer since it needs to detach the volume twice: additional one for data copy job
		waitRestoreTimeout = 240
	}
	origEtcd = etcdClusterWithRestore(origEtcd, &spec.RestorePolicy{
		BackupClusterName: testEtcd.Name,
		StorageType:       spec.BackupStorageTypePersistentVolume,
	})
	testEtcd, err = createEtcdCluster(f, etcdClusterWithBackup(origEtcd, makeBackupPolicy(true)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
			t.Fatal(err)
		}
	}()
	names, err = waitUntilSizeReached(f, testEtcd.Name, 3, waitRestoreTimeout)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	pod, err = f.KubeClient.Pods(f.Namespace.Name).Get(names[0])
	if err != nil {
		t.Fatal(err)
	}
	etcdcli, err = createEtcdClient(fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.Get(ctx, etcdKeyFoo)
	cancel()
	etcdcli.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Kvs) != 1 {
		t.Errorf("want only 1 key result, get %d", len(resp.Kvs))
	} else {
		val := string(resp.Kvs[0].Value)
		if val != etcdValBar {
			t.Errorf("value want = '%s', get = '%s'", etcdValBar, val)
		}
	}
}
