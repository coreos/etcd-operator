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

package upgradetest

import (
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
)

func TestResize(t *testing.T) {
	defer func() {
		err := testF.DeleteOperator()
		if err != nil {
			t.Fatal(err)
		}
	}()
	err := testF.CreateOperator()
	if err != nil {
		t.Fatal(err)
	}
	if err = k8sutil.WaitEtcdTPRReady(testF.KubeCli.CoreV1().RESTClient(), 3*time.Second, 30*time.Second, testF.KubeNS); err != nil {
		t.Fatalf("failed to see cluster TPR get created in time: %v", err)
	}

	testClus, err := e2eutil.CreateCluster(t, testF.KubeCli, testF.KubeNS, e2eutil.NewCluster("upgrade-test-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteCluster(t, testF.KubeCli, testClus, &e2eutil.StorageCheckerOptions{}); err != nil {
			t.Fatal(err)
		}
	}()
	_, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 3, 60*time.Second, testClus)
	if err != nil {
		t.Fatal(err)
	}
	err = testF.UpgradeOperator()
	if err != nil {
		t.Fatal(err)
	}

	testClus, err = k8sutil.GetClusterTPRObject(testF.KubeCli.CoreV1().RESTClient(), testF.KubeNS, testClus.Metadata.Name)
	if err != nil {
		t.Fatal(err)
	}

	updateFunc := func(cl *spec.Cluster) {
		cl.Spec.Size = 5
	}
	if _, err := e2eutil.UpdateCluster(testF.KubeCli, testClus, 10, updateFunc); err != nil {
		t.Fatal(err)
	}
	_, err = e2eutil.WaitUntilSizeReached(t, testF.KubeCli, 5, 60*time.Second, testClus)
	if err != nil {
		t.Fatal(err)
	}
}
