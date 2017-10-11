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

package v1beta2

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EtcdRestoreList is a list of EtcdRestore.
type EtcdRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []EtcdBackup `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EtcdRestore represents a Kubernetes EtcdRestore Custom Resource.
type EtcdRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              RestoreSpec   `json:"spec"`
	Status            RestoreStatus `json:"status,omitempty"`
}

// RestoreSpec defines how to restore an etcd cluster from existing backup.
type RestoreSpec struct {
	// ClusterSpec defines the same spec that etcd operator will run later.
	// using this spec, restore operator will create the seed etcd member that
	// etcd operator will pick up later.
	ClusterSpec ClusterSpec `json:"clusterSpec"`
	// BackupSpec defines the same spec that backup operator uses to save the backup.
	// restore operator will have the same logic as backup operator to discover
	// any existing backups and find the one with largest revision.
	BackupSpec EtcdBackupSpec `json:"backupSpec"`
}

// RestoreStatus reports the status of this restore operation.
type RestoreStatus struct {
	// Succeeded indicates if the backup has Succeeded.
	Succeeded bool `json:"succeeded"`
	// Reason indicates the reason for any backup related failures.
	Reason string `json:"reason"`
}
