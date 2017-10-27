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
	"os"

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

func NewS3BackupPolicy(cleanup bool) *api.BackupPolicy {
	return &api.BackupPolicy{
		BackupIntervalInSecond: 60 * 60,
		MaxBackups:             5,
		StorageType:            api.BackupStorageTypeS3,
		StorageSource: api.StorageSource{
			S3: &api.S3Source{
				S3Bucket:  os.Getenv("TEST_S3_BUCKET"),
				AWSSecret: os.Getenv("TEST_AWS_SECRET"),
			},
		},
		AutoDelete: cleanup,
	}
}

// NewS3Backup creates a EtcdBackup object using clusterName.
func NewS3Backup(clusterName string) *api.EtcdBackup {
	return &api.EtcdBackup{
		TypeMeta: metav1.TypeMeta{
			Kind:       api.EtcdBackupResourceKind,
			APIVersion: api.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: clusterName,
		},
		Spec: api.BackupSpec{
			ClusterName: clusterName,
			StorageType: api.BackupStorageTypeS3,
			BackupStorageSource: api.BackupStorageSource{
				S3: &api.S3Source{
					S3Bucket:  os.Getenv("TEST_S3_BUCKET"),
					AWSSecret: os.Getenv("TEST_AWS_SECRET"),
				},
			},
		},
	}
}

func NewPVBackupPolicy(cleanup bool, storageClass string) *api.BackupPolicy {
	return &api.BackupPolicy{
		BackupIntervalInSecond: 60 * 60,
		MaxBackups:             5,
		StorageType:            api.BackupStorageTypePersistentVolume,
		StorageSource: api.StorageSource{
			PV: &api.PVSource{
				VolumeSizeInMB: 512,
				StorageClass:   storageClass,
			},
		},
		AutoDelete: cleanup,
	}
}

func ClusterWithBackup(cl *api.EtcdCluster, backupPolicy *api.BackupPolicy) *api.EtcdCluster {
	cl.Spec.Backup = backupPolicy
	return cl
}

func ClusterWithRestore(cl *api.EtcdCluster, restorePolicy *api.RestorePolicy) *api.EtcdCluster {
	cl.Spec.Restore = restorePolicy
	return cl
}

func ClusterWithVersion(cl *api.EtcdCluster, version string) *api.EtcdCluster {
	cl.Spec.Version = version
	return cl
}

func ClusterWithSelfHosted(cl *api.EtcdCluster, sh *api.SelfHostedPolicy) *api.EtcdCluster {
	cl.Spec.SelfHosted = sh
	return cl
}

// NameLabelSelector returns a label selector of the form name=<name>
func NameLabelSelector(name string) map[string]string {
	return map[string]string{"name": name}
}
