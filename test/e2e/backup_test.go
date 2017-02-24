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
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

func TestBackupStatus(t *testing.T) {
	f := framework.Global

	bp := newBackupPolicyPV()
	bp.CleanupBackupsOnClusterDelete = true
	testEtcd, err := createCluster(t, f, etcdClusterWithBackup(newClusterSpec("test-etcd-", 1), bp))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(t, f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	_, err = waitUntilSizeReached(t, f, testEtcd.Metadata.Name, 1, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create 1 members etcd cluster: %v", err)
	}
	if err := waitBackupPodUp(f, testEtcd.Metadata.Name, 60*time.Second); err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}
	if err := makeBackup(f, testEtcd.Metadata.Name); err != nil {
		t.Fatalf("fail to make backup: %v", err)
	}

	err = retryutil.Retry(5*time.Second, 12, func() (done bool, err error) {
		c, err := k8sutil.GetClusterTPRObject(f.KubeClient.CoreV1().RESTClient(), f.Namespace, testEtcd.Metadata.Name)
		if err != nil {
			t.Fatalf("faied to get cluster spec: %v", err)
		}
		bs := c.Status.BackupServiceStatus
		if bs == nil {
			return false, nil
		}
		if bs.Backups != 1 {
			return true, fmt.Errorf("backups = %d, want 1", bs.Backups)
		}
		return true, nil
	})

	if err != nil {
		t.Error(err)
	}
}
