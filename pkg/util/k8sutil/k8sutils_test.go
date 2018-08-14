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

package k8sutil

import (
	"github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"k8s.io/api/core/v1"
	"testing"
)

func TestDefaultBusyboxImageName(t *testing.T) {
	policy := &api.PodPolicy{}
	image := imageNameBusybox(policy)
	expected := defaultBusyboxImage
	if image != expected {
		t.Errorf("expect image=%s, get=%s", expected, image)
	}
}

func TestDefaultNilBusyboxImageName(t *testing.T) {
	image := imageNameBusybox(nil)
	expected := defaultBusyboxImage
	if image != expected {
		t.Errorf("expect image=%s, get=%s", expected, image)
	}
}

func TestSetBusyboxImageName(t *testing.T) {
	policy := &api.PodPolicy{
		BusyboxImage: "myRepo/busybox:1.3.2",
	}
	image := imageNameBusybox(policy)
	expected := "myRepo/busybox:1.3.2"
	if image != expected {
		t.Errorf("expect image=%s, get=%s", expected, image)
	}
}

func TestNewEtcdPod(t *testing.T) {
	m := &etcdutil.Member{
		Name:         "name-test",
		Namespace:    "test",
		ID:           uint64(0),
		SecurePeer:   false,
		SecureClient: false,
	}
	cs := v1beta2.ClusterSpec{
		Size:       3,
		Repository: "quay.io/coreos/etcd",
		Version:    "",
		Pod: &v1beta2.PodPolicy{
			InitContainers: []v1.Container{
				{
					Name:  "initTest01",
					Image: "quay.io/coreos/etcd",
				},
				{
					Name:  "initTest02",
					Image: "quay.io/coreos/etcd",
				},
			},
			SideCarContainers: []v1.Container{
				{
					Name:  "sideCar01",
					Image: "quay.io/coreos/etcd",
				},
				{
					Name:  "sideCar02",
					Image: "quay.io/coreos/etcd",
				},
			},
		},
	}
	pod := newEtcdPod(m, []string{"test-cluster-name"}, "test-cluster-name", "emptyState", "emptyToken", cs)
	if len(pod.Spec.InitContainers) != 3 {
		t.Error("init container failed")
	}
	if len(pod.Spec.Containers) != 3 {
		t.Error("sideCar container failed")
	}
}
