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
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"k8s.io/kubernetes/pkg/api"
)

func TestEtcdUpgrade(t *testing.T) {
	f := framework.Global
	origEtcd := makeEtcdCluster("test-etcd-", 3)
	origEtcd = etcdClusterWithVersion(origEtcd, "v3.0.12")
	testEtcd, err := createEtcdCluster(f, origEtcd)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd.Name); err != nil {
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
