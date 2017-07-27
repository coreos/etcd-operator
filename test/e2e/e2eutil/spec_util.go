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

	"github.com/coreos/etcd-operator/pkg/spec"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewCluster(genName string, size int) *spec.EtcdCluster {
	return &spec.EtcdCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       spec.CRDResourceKind,
			APIVersion: spec.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: genName,
		},
		Spec: spec.ClusterSpec{
			Size: size,
		},
	}
}

func NewS3BackupPolicy(cleanup bool) *spec.BackupPolicy {
	return &spec.BackupPolicy{
		BackupIntervalInSecond: 60 * 60,
		MaxBackups:             5,
		StorageType:            spec.BackupStorageTypeS3,
		StorageSource: spec.StorageSource{
			S3: &spec.S3Source{
				S3Bucket:  os.Getenv("TEST_S3_BUCKET"),
				AWSSecret: os.Getenv("TEST_AWS_SECRET"),
			},
		},
		AutoDelete: cleanup,
	}
}

func NewOperatorS3BackupPolicy(cleanup bool) *spec.BackupPolicy {
	return &spec.BackupPolicy{
		BackupIntervalInSecond: 60 * 60,
		MaxBackups:             5,
		StorageType:            spec.BackupStorageTypeS3,
		AutoDelete:             cleanup,
	}
}

func NewPVBackupPolicy(cleanup bool) *spec.BackupPolicy {
	return &spec.BackupPolicy{
		BackupIntervalInSecond: 60 * 60,
		MaxBackups:             5,
		StorageType:            spec.BackupStorageTypePersistentVolume,
		StorageSource: spec.StorageSource{
			PV: &spec.PVSource{
				VolumeSizeInMB: 512,
			},
		},
		AutoDelete: cleanup,
	}
}

func ClusterWithBackup(cl *spec.EtcdCluster, backupPolicy *spec.BackupPolicy) *spec.EtcdCluster {
	cl.Spec.Backup = backupPolicy
	return cl
}

func ClusterWithRestore(cl *spec.EtcdCluster, restorePolicy *spec.RestorePolicy) *spec.EtcdCluster {
	cl.Spec.Restore = restorePolicy
	return cl
}

func ClusterWithVersion(cl *spec.EtcdCluster, version string) *spec.EtcdCluster {
	cl.Spec.Version = version
	return cl
}

func ClusterWithSelfHosted(cl *spec.EtcdCluster, sh *spec.SelfHostedPolicy) *spec.EtcdCluster {
	cl.Spec.SelfHosted = sh
	return cl
}

// NameLabelSelector returns a label selector of the form name=<name>
func NameLabelSelector(name string) map[string]string {
	return map[string]string{"name": name}
}
