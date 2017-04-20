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

package cluster

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
)

func TestNewBackupManagerWithNonePVProvisioner(t *testing.T) {
	cfg := Config{PVProvisioner: constants.PVProvisionerNone}
	cl := &spec.Cluster{
		Metadata: metav1.ObjectMeta{Name: "testing"},
		Spec: spec.ClusterSpec{
			Backup: &spec.BackupPolicy{
				StorageType: spec.BackupStorageTypePersistentVolume,
				StorageSource: spec.StorageSource{
					PV: &spec.PVSource{VolumeSizeInMB: 512},
				},
				MaxBackups: 1,
			},
		},
	}
	_, err := newBackupManager(cfg, cl, nil)
	if err != errNoPVForBackup {
		t.Errorf("expect err=%v, get=%v", errNoPVForBackup, err)
	}
}

func TestNewBackupManagerWithoutS3Config(t *testing.T) {
	cfg := Config{}
	cl := &spec.Cluster{
		Metadata: metav1.ObjectMeta{Name: "testing"},
		Spec: spec.ClusterSpec{
			Backup: &spec.BackupPolicy{
				StorageType: spec.BackupStorageTypeS3,
				MaxBackups:  1,
			},
		},
	}
	_, err := newBackupManager(cfg, cl, nil)
	if err != errNoS3ConfigForBackup {
		t.Errorf("expect err=%v, get=%v", errNoS3ConfigForBackup, err)
	}
}
