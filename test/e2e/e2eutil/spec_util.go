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

package e2eutil

import (
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewCluster(genName string, size int) *api.EtcdCluster {
	return &api.EtcdCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       api.EtcdClusterResourceKind,
			APIVersion: api.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: genName,
		},
		Spec: api.ClusterSpec{
			Size: size,
		},
	}
}

// NewS3Backup creates a EtcdBackup object using clusterName.
func NewS3Backup(endpoints []string, clusterName, path, secret, clientTLSSecret string) *api.EtcdBackup {
	return &api.EtcdBackup{
		TypeMeta: metav1.TypeMeta{
			Kind:       api.EtcdBackupResourceKind,
			APIVersion: api.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: clusterName,
		},
		Spec: api.BackupSpec{
			EtcdEndpoints:   endpoints,
			StorageType:     api.BackupStorageTypeS3,
			ClientTLSSecret: clientTLSSecret,
			BackupSource: api.BackupSource{
				S3: &api.S3BackupSource{
					Path:           path,
					AWSSecret:      secret,
					ForcePathStyle: false,
				},
			},
		},
	}
}

// NewS3RestoreSource returns an S3RestoreSource with the specified path and secret
func NewS3RestoreSource(path, awsSecret string) *api.S3RestoreSource {
	return &api.S3RestoreSource{
		Path:      path,
		AWSSecret: awsSecret,
	}
}

// NewEtcdRestore returns an EtcdRestore CR with the specified RestoreSource
func NewEtcdRestore(clusterName string, size int, restoreSource api.RestoreSource, backupStorageType api.BackupStorageType) *api.EtcdRestore {
	return &api.EtcdRestore{
		TypeMeta: metav1.TypeMeta{
			Kind:       api.EtcdRestoreResourceKind,
			APIVersion: api.SchemeGroupVersion.String(),
		},
		// The EtcdRestore CR name must be the same as the EtcdClusterRef name
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
		Spec: api.RestoreSpec{
			EtcdCluster: api.EtcdClusterRef{
				Name: clusterName,
			},
			BackupStorageType: backupStorageType,
			RestoreSource:     restoreSource,
		},
	}
}

// ClusterCRWithTLS adds TLSPolicy to the passing in cluster CR.
func ClusterCRWithTLS(cl *api.EtcdCluster, memberPeerTLSSecret, memberServerTLSSecret, operatorClientTLSSecret string) {
	cl.Spec.TLS = &api.TLSPolicy{
		Static: &api.StaticTLS{
			Member: &api.MemberSecret{
				PeerSecret:   memberPeerTLSSecret,
				ServerSecret: memberServerTLSSecret,
			},
			OperatorSecret: operatorClientTLSSecret,
		},
	}
}

func ClusterWithVersion(cl *api.EtcdCluster, version string) *api.EtcdCluster {
	cl.Spec.Version = version
	return cl
}

// NameLabelSelector returns a label selector of the form name=<name>
func NameLabelSelector(name string) map[string]string {
	return map[string]string{"name": name}
}
