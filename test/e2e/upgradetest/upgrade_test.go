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
	"fmt"
	"math/rand"
	"testing"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newOperatorName() string {
	suffix := fmt.Sprintf("-%d", rand.Uint64())
	return "etcd-operator" + suffix
}

func TestResize(t *testing.T) {
	name := newOperatorName()
	err := testF.CreateOperator(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := testF.DeleteOperator(name)
		if err != nil {
			t.Fatal(err)
		}
	}()
	err = e2eutil.WaitUntilOperatorReady(testF.KubeCli, testF.KubeNS, name)
	if err != nil {
		t.Fatal(err)
	}

	testClus, err := e2eutil.CreateCluster(t, testF.CRClient, testF.KubeNS, e2eutil.NewCluster("upgrade-test-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteCluster(t, testF.CRClient, testF.KubeCli, testClus); err != nil {
			t.Fatal(err)
		}
	}()
	_, err = e2eutil.WaitUntilSizeReached(t, testF.CRClient, 3, 6, testClus)
	if err != nil {
		t.Fatal(err)
	}
	err = testF.UpgradeOperator(name)
	if err != nil {
		t.Fatal(err)
	}
	testClus, err = testF.CRClient.EtcdV1beta2().EtcdClusters(testF.KubeNS).Get(testClus.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	updateFunc := func(cl *api.EtcdCluster) {
		cl.Spec.Size = 5
	}
	_, err = e2eutil.UpdateCluster(testF.CRClient, testClus, 10, updateFunc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e2eutil.WaitUntilSizeReached(t, testF.CRClient, 5, 6, testClus)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHealOneMemberForOldCluster(t *testing.T) {
	name := newOperatorName()
	err := testF.CreateOperator(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := testF.DeleteOperator(name)
		if err != nil {
			t.Fatal(err)
		}
	}()
	err = e2eutil.WaitUntilOperatorReady(testF.KubeCli, testF.KubeNS, name)
	if err != nil {
		t.Fatal(err)
	}

	testEtcd, err := e2eutil.CreateCluster(t, testF.CRClient, testF.KubeNS, e2eutil.NewCluster("upgrade-test-", 3))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteCluster(t, testF.CRClient, testF.KubeCli, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()
	names, err := e2eutil.WaitUntilSizeReached(t, testF.CRClient, 3, 6, testEtcd)
	if err != nil {
		t.Fatal(err)
	}

	err = testF.UpgradeOperator(name)
	if err != nil {
		t.Fatal(err)
	}

	err = e2eutil.KillMembers(testF.KubeCli, testF.KubeNS, names[2])
	if err != nil {
		t.Fatal(err)
	}

	remaining, err := e2eutil.WaitUntilMembersWithNamesDeleted(t, testF.CRClient, 3, testEtcd, names[2])
	if err != nil {
		t.Fatalf("failed to see members (%v) be deleted in time: %v", remaining, err)
	}

	_, err = e2eutil.WaitUntilSizeReached(t, testF.CRClient, 3, 6, testEtcd)
	if err != nil {
		t.Fatalf("failed to heal one member: %v", err)
	}
}
