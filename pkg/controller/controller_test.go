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

package controller

import (
	"strings"
	"testing"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

func TestHandleClusterEventUpdateFailedCluster(t *testing.T) {
	c := New(Config{})

	clus := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Status: api.ClusterStatus{
			Phase: api.ClusterPhaseFailed,
		},
	}
	e := &Event{
		Type:   watch.Modified,
		Object: clus,
	}
	_, err := c.handleClusterEvent(e)
	prefix := "ignore failed cluster"
	if !strings.HasPrefix(err.Error(), prefix) {
		t.Errorf("expect err='%s...', get=%v", prefix, err)
	}
}

func TestHandleClusterEventDeleteFailedCluster(t *testing.T) {
	c := New(Config{})
	name := "tests"
	clus := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: api.ClusterStatus{
			Phase: api.ClusterPhaseFailed,
		},
	}
	e := &Event{
		Type:   watch.Deleted,
		Object: clus,
	}

	c.clusters[getNamespacedName(clus)] = &cluster.Cluster{}

	if _, err := c.handleClusterEvent(e); err != nil {
		t.Fatal(err)
	}

	if c.clusters[getNamespacedName(clus)] != nil {
		t.Errorf("failed cluster not cleaned up after delete event, cluster struct: %v", c.clusters[getNamespacedName(clus)])
	}
}

func TestHandleClusterEventClusterwide(t *testing.T) {
	c := New(Config{ClusterWide: true})

	clus := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "a",
			Annotations: map[string]string{
				"etcd.database.coreos.com/scope": "clusterwide",
			},
		},
	}
	e := &Event{
		Type:   watch.Modified,
		Object: clus,
	}
	if ignored, _ := c.handleClusterEvent(e); ignored {
		t.Errorf("cluster shouldn't be ignored")
	}
}

func TestHandleClusterEventClusterwideIgnored(t *testing.T) {
	c := New(Config{ClusterWide: true})

	clus := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	e := &Event{
		Type:   watch.Modified,
		Object: clus,
	}
	if ignored, _ := c.handleClusterEvent(e); !ignored {
		t.Errorf("cluster should be ignored")
	}
}

func TestHandleClusterEventClusterwideAddTwoCR(t *testing.T) {
	c := New(Config{ClusterWide: true})
	clusA := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "a",
		},
	}
	clusB := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "b",
		},
	}
	e := &Event{
		Type:   watch.Added,
		Object: clusA,
	}
	ignored, errA := c.handleClusterEvent(e)
	if !ignored && errA != nil {
		t.Errorf("cluster should be Added")
	}
	e.Object = clusB
	ignored, errB := c.handleClusterEvent(e)
	if !ignored && errB != nil {
		t.Errorf("cluster should be Added")
	}
}

func TestHandleClusterEventNamespacedIgnored(t *testing.T) {
	c := New(Config{})

	clus := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				"etcd.database.coreos.com/scope": "clusterwide",
			},
		},
	}
	e := &Event{
		Type:   watch.Modified,
		Object: clus,
	}
	if ignored, _ := c.handleClusterEvent(e); !ignored {
		t.Errorf("cluster should be ignored")
	}
}

func TestHandleClusterEventWithLongClusterName(t *testing.T) {
	c := New(Config{})

	clus := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-etcd-cluster123456789123456789123456789123456789123456",
		},
	}
	e := &Event{
		Type:   watch.Added,
		Object: clus,
	}
	if ignored, err := c.handleClusterEvent(e); !ignored {
		if err == nil {
			t.Errorf("err should not be nil")
		}
	} else {
		t.Errorf("cluster should not be ignored")
	}
}
