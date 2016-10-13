// Copyright 2016 The kube-etcd-controller Authors
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

package spec

import (
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
)

type EtcdCluster struct {
	unversioned.TypeMeta `json:",inline"`
	api.ObjectMeta       `json:"metadata,omitempty"`
	Spec                 ClusterSpec `json:"spec"`
}

type ClusterSpec struct {
	// Size is the expected size of the etcd cluster.
	// The controller will eventually make the size of the running
	// cluster equal to the expected size.
	// The vaild range of the size is from 1 to 7.
	Size int `json:"size"`

	// NodeSelector specifies a map of key-value pairs. For the pod to be eligible
	// to run on a node, the node must have each of the indicated key-value pairs as
	// labels.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// AntiAffinity determines if the controller tries to avoid putting
	// the etcd members in the same cluster onto the same node.
	AntiAffinity bool `json:"antiAffinity"`
	// Version is the expected version of the etcd cluster.
	// The controller will eventually make the etcd cluster version
	// equal to the expected version.
	Version string `json:"version"`
	// Backup is the backup policy for the etcd cluster.
	// There is no backup by default.
	Backup *BackupPolicy `json:"backup,omitempty"`
	// HostNetwork determines if the etcd pods should be run
	// in the host network namespace.
	HostNetwork bool `json:"hostNetwork,omitempty"`
	// Seed specifies a seed member for the cluster.
	// If there is no seed member, a completely new cluster will be created.
	// There is no seed member by default.
	Seed *SeedPolicy `json:"seed,omitempty"`
}

type SeedPolicy struct {
	// The client endpoints of the seed member.
	MemberClientEndpoints []string
	// RemoveDelay specifies the delay to remove the original seed member from the
	// cluster in seconds.
	// The seed member will be removed in 30 second by default. Minimum delay is 10 seconds.
	RemoveDelay int
}
