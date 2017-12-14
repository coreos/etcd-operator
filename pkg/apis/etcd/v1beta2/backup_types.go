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

const (
	BackupStorageTypeS3 BackupStorageType = "S3"

	AWSSecretCredentialsFileName = "credentials"
	AWSSecretConfigFileName      = "config"
)

type BackupStorageType string

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EtcdBackupList is a list of EtcdBackup.
type EtcdBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []EtcdBackup `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EtcdBackup represents a Kubernetes EtcdBackup Custom Resource.
type EtcdBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BackupSpec     `json:"spec"`
	Status            BackupCRStatus `json:"status,omitempty"`
}

// BackupSpec contains a backup specification for an etcd cluster.
type BackupSpec struct {
	// EtcdEndpoints specifies the endpoints of an etcd cluster.
	// When multiple endpoints are given, the backup operator retrieves
	// the backup from the endpoint that has the most up-to-date state.
	// The given endpoints must belong to the same etcd cluster.
	EtcdEndpoints []string `json:"etcdEndpoints,omitempty"`
	// StorageType is the etcd backup storage type.
	// We need this field because CRD doesn't support validation against invalid fields
	// and we cannot verify invalid backup storage source.
	StorageType BackupStorageType `json:"storageType"`
	// BackupSource is the backup storage source.
	BackupSource `json:",inline"`
	// ClientTLSSecret is the secret containing the etcd TLS client certs and
	// must contain the following data items:
	// data:
	//    "etcd-client.crt": <pem-encoded-cert>
	//    "etcd-client.key": <pem-encoded-key>
	//    "etcd-client-ca.crt": <pem-encoded-ca-cert>
	ClientTLSSecret string `json:"clientTLSSecret,omitempty"`
}

// BackupSource contains the supported backup sources.
type BackupSource struct {
	// S3 defines the S3 backup source spec.
	S3 *S3BackupSource `json:"s3,omitempty"`
}

// BackupCRStatus represents the status of the EtcdBackup Custom Resource.
type BackupCRStatus struct {
	// Succeeded indicates if the backup has Succeeded.
	Succeeded bool `json:"succeeded"`
	// Reason indicates the reason for any backup related failures.
	Reason string `json:"Reason,omitempty"`
}

// S3BackupSource provides the spec how to store backups on S3.
type S3BackupSource struct {
	// Path is the full s3 path where the backup is saved.
	// The format of the path must be: "<s3-bucket-name>/<path-to-backup-file>"
	// e.g: "mybucket/etcd.backup"
	Path string `json:"path"`

	// The name of the secret object that stores the AWS credential and config files.
	// The file name of the credential MUST be 'credentials'.
	// The file name of the config MUST be 'config'.
	// The profile to use in both files will be 'default'.
	//
	// AWSSecret overwrites the default etcd operator wide AWS credential and config.
	AWSSecret string `json:"awsSecret"`
}
