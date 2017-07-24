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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/coreos/etcd-operator/pkg/cluster"
	"github.com/coreos/etcd-operator/pkg/spec"
)

func TestHandleClusterEventUpdateFailedCluster(t *testing.T) {
	c := New(Config{})

	clus := &spec.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Status: spec.ClusterStatus{
			Phase: spec.ClusterPhaseFailed,
		},
	}
	e := &Event{
		Type:   watch.Modified,
		Object: clus,
	}
	err := c.handleClusterEvent(e)
	prefix := "ignore failed cluster"
	if !strings.HasPrefix(err.Error(), prefix) {
		t.Errorf("expect err='%s...', get=%v", prefix, err)
	}
}

func TestHandleClusterEventDeleteFailedCluster(t *testing.T) {
	c := New(Config{})
	name := "tests"
	clus := &spec.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: spec.ClusterStatus{
			Phase: spec.ClusterPhaseFailed,
		},
	}
	e := &Event{
		Type:   watch.Deleted,
		Object: clus,
	}

	c.clusters[name] = &cluster.Cluster{}
	c.clusterRVs[name] = "123"

	if err := c.handleClusterEvent(e); err != nil {
		t.Fatal(err)
	}

	if c.clusters[name] != nil || c.clusterRVs[name] != "" {
		t.Errorf("failed cluster not cleaned up after delete event, cluster struct: %v, RV: %s", c.clusters[name], c.clusterRVs[name])
	}
}
