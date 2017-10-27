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

	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestEtcdBackupOperatorForS3Backup tests if etcd backup operator can save etcd backup to S3.
func TestEtcdBackupOperatorForS3Backup(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	f := framework.Global
	testEtcd, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, e2eutil.NewCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := e2eutil.WaitUntilSizeReached(t, f.CRClient, 3, 6, testEtcd); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	eb, err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Create(e2eutil.NewS3Backup(testEtcd.Name))
	if err != nil {
		t.Fatalf("failed to create etcd backup cr: %v", err)
	}
	defer func() {
		if err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Delete(eb.Name, nil); err != nil {
			t.Fatalf("failed to delete etcd backup cr: %v", err)
		}
	}()

	err = retryutil.Retry(time.Second, 10, func() (bool, error) {
		reb, err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Get(eb.Name, v1.GetOptions{})
		if err != nil {
			return true, fmt.Errorf("failed to retrieve backup CR: %v", err)
		}
		if reb.Status.Succeeded {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to verify backup status: %v", err)
	}
	// TODO: also check if backup is there on S3 path.
}
