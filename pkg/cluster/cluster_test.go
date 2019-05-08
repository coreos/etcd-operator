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

package cluster

import (
	"testing"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// When EtcdCluster update event happens, local object ref should be updated.
func TestUpdateEventUpdateLocalClusterObj(t *testing.T) {
	oldVersion := "123"
	newVersion := "321"

	oldObj := &api.EtcdCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: api.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: oldVersion,
			Name:            "test",
			Namespace:       metav1.NamespaceDefault,
		},
	}
	newObj := oldObj.DeepCopy()
	newObj.ResourceVersion = newVersion

	c := &Cluster{
		cluster: oldObj,
	}
	e := &clusterEvent{
		typ:     eventModifyCluster,
		cluster: newObj,
	}

	err := c.handleUpdateEvent(e)
	if err != nil {
		t.Fatal(err)
	}
	if c.cluster.ResourceVersion != newVersion {
		t.Errorf("expect version=%s, get=%s", newVersion, c.cluster.ResourceVersion)
	}
}

func TestNewLongClusterName(t *testing.T) {
	clus := &api.EtcdCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: api.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-etcd-cluster123456789123456789123456789123456789123456",
			Namespace: metav1.NamespaceDefault,
		},
	}
	clus.SetClusterName("example-etcd-cluster123456789123456789123456789123456789123456")
	if c := New(Config{}, clus); c != nil {
		t.Errorf("expect c to be nil")
	}
}
