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

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

func TestCreateCluster(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	f := framework.Global
	testEtcd, err := e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, e2eutil.NewCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteCluster(t, f.KubeClient, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 60*time.Second, testEtcd); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
}

// TestPauseControl tests the user can pause the operator from controlling
// an etcd cluster.
func TestPauseControl(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	f := framework.Global
	testEtcd, err := e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, e2eutil.NewCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteCluster(t, f.KubeClient, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	names, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 60*time.Second, testEtcd)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	updateFunc := func(cl *spec.Cluster) {
		cl.Spec.Paused = true
	}
	if testEtcd, err = e2eutil.UpdateCluster(f.KubeClient, testEtcd, 10, updateFunc); err != nil {
		t.Fatalf("failed to pause control: %v", err)
	}

	// TODO: this is used to wait for the TPR to be updated.
	// TODO: make this wait for reliable
	time.Sleep(5 * time.Second)

	if err := e2eutil.KillMembers(f.KubeClient, f.Namespace, names[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 2, 10*time.Second, testEtcd); err != nil {
		t.Fatalf("failed to wait for killed member to die: %v", err)
	}
	if _, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 10*time.Second, testEtcd); err == nil {
		t.Fatalf("cluster should not be recovered: control is paused")
	}

	updateFunc = func(cl *spec.Cluster) {
		cl.Spec.Paused = false
	}
	if testEtcd, err = e2eutil.UpdateCluster(f.KubeClient, testEtcd, 10, updateFunc); err != nil {
		t.Fatalf("failed to resume control: %v", err)
	}

	if _, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 60*time.Second, testEtcd); err != nil {
		t.Fatalf("failed to resize to 3 members etcd cluster: %v", err)
	}
}

func TestEtcdUpgrade(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	f := framework.Global
	origEtcd := e2eutil.NewCluster("test-etcd-", 3)
	origEtcd = e2eutil.ClusterWithVersion(origEtcd, "3.0.16")
	testEtcd, err := e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, origEtcd)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteCluster(t, f.KubeClient, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	err = e2eutil.WaitSizeAndVersionReached(t, f.KubeClient, "3.0.16", 3, 60*time.Second, testEtcd)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	updateFunc := func(cl *spec.Cluster) {
		cl = e2eutil.ClusterWithVersion(cl, "3.1.8")
	}
	if _, err := e2eutil.UpdateCluster(f.KubeClient, testEtcd, 10, updateFunc); err != nil {
		t.Fatalf("fail to update cluster version: %v", err)
	}

	err = e2eutil.WaitSizeAndVersionReached(t, f.KubeClient, "3.1.8", 3, 60*time.Second, testEtcd)
	if err != nil {
		t.Fatalf("failed to wait new version etcd cluster: %v", err)
	}
}
