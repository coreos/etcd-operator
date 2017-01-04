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

	"github.com/coreos/etcd-operator/test/e2e/framework"
)

func TestResize(t *testing.T) {
	t.Run("resize etcd cluster", func(t *testing.T) {
		t.Run("resize 3->5", testResizeCluster3to5)
		t.Run("resize 5->3", testResizeCluster5to3)
	})
}

func testResizeCluster3to5(t *testing.T) {
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
	fmt.Println("reached to 3 members cluster")

	testEtcd.Spec.Size = 5
	if _, err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 5, 60*time.Second); err != nil {
		t.Fatalf("failed to resize to 5 members etcd cluster: %v", err)
	}
}

func testResizeCluster5to3(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeEtcdCluster("test-etcd-", 5))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 5, 90*time.Second); err != nil {
		t.Fatalf("failed to create 5 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 5 members cluster")

	testEtcd.Spec.Size = 3
	if _, err := updateEtcdCluster(f, testEtcd); err != nil {
		t.Fatal(err)
	}

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 60*time.Second); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}
