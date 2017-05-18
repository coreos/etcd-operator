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
	"strings"

	"github.com/coreos/etcd-operator/pkg/spec"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewCluster(genName string, size int) *spec.Cluster {
	return &spec.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       strings.Title(spec.TPRKind),
			APIVersion: spec.TPRGroup + "/" + spec.TPRVersion,
		},
		Metadata: metav1.ObjectMeta{
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
		CleanupBackupsOnClusterDelete: cleanup,
	}
}

func NewOperatorS3BackupPolicy(cleanup bool) *spec.BackupPolicy {
	return &spec.BackupPolicy{
		BackupIntervalInSecond:        60 * 60,
		MaxBackups:                    5,
		StorageType:                   spec.BackupStorageTypeS3,
		CleanupBackupsOnClusterDelete: cleanup,
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
		CleanupBackupsOnClusterDelete: cleanup,
	}
}

func ClusterWithBackup(cl *spec.Cluster, backupPolicy *spec.BackupPolicy) *spec.Cluster {
	cl.Spec.Backup = backupPolicy
	return cl
}

func ClusterWithRestore(cl *spec.Cluster, restorePolicy *spec.RestorePolicy) *spec.Cluster {
	cl.Spec.Restore = restorePolicy
	return cl
}

func ClusterWithVersion(cl *spec.Cluster, version string) *spec.Cluster {
	cl.Spec.Version = version
	return cl
}

func ClusterWithSelfHosted(cl *spec.Cluster, sh *spec.SelfHostedPolicy) *spec.Cluster {
	cl.Spec.SelfHosted = sh
	return cl
}
