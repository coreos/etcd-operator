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
	"os"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"k8s.io/kubernetes/pkg/api"
)

func TestBasic(t *testing.T) {
	t.Run("basic test", func(t *testing.T) {
		t.Run("create cluster", testCreateCluster)
		t.Run("pause control", testPauseControl)
		t.Run("upgrade cluster", testEtcdUpgrade)
	})
}

func testCreateCluster(t *testing.T) {
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

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60*time.Second); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
}

// testPauseControl tests the user can pause the operator from controlling
// an etcd cluster.
func testPauseControl(t *testing.T) {
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

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 2, 30*time.Second); err != nil {
		t.Fatalf("failed to wait for killed member to die: %v", err)
	}
	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 30*time.Second); err == nil {
		t.Fatalf("cluster should not be recovered: control is paused")
	}

	testEtcd.Spec.Paused = false
	if _, err = updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatalf("failed to resume control: %v", err)
	}

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60*time.Second); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

func testEtcdUpgrade(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	f := framework.Global
	origEtcd := makeEtcdCluster("test-etcd-", 3)
	origEtcd = etcdClusterWithVersion(origEtcd, "v3.0.12")
	testEtcd, err := createEtcdCluster(f, origEtcd)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	_, err = waitSizeReachedWithFilter(f, testEtcd.Name, 3, 60*time.Second, func(pod *api.Pod) bool {
		return k8sutil.GetEtcdVersion(pod) == "v3.0.12"
	})
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	testEtcd = etcdClusterWithVersion(testEtcd, "v3.1.0-alpha.1")

	if _, err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatalf("fail to update cluster version: %v", err)
	}

	_, err = waitSizeReachedWithFilter(f, testEtcd.Name, 3, 60*time.Second, func(pod *api.Pod) bool {
		return k8sutil.GetEtcdVersion(pod) == "v3.1.0-alpha.1"
	})
	if err != nil {
		t.Fatalf("failed to wait new version etcd cluster: %v", err)
	}
}
