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

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta1"
	"github.com/coreos/etcd-operator/pkg/util/constants"
)

func TestNewBackupManagerWithNonePVProvisioner(t *testing.T) {
	cfg := Config{PVProvisioner: constants.PVProvisionerNone}
	cl := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "testing"},
		Spec: api.ClusterSpec{
			Backup: &api.BackupPolicy{
				StorageType: api.BackupStorageTypePersistentVolume,
				StorageSource: api.StorageSource{
					PV: &api.PVSource{VolumeSizeInMB: 512},
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
	cl := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "testing"},
		Spec: api.ClusterSpec{
			Backup: &api.BackupPolicy{
				StorageType: api.BackupStorageTypeS3,
				MaxBackups:  1,
			},
		},
	}
	_, err := newBackupManager(cfg, cl, nil)
	if err != errNoS3ConfigForBackup {
		t.Errorf("expect err=%v, get=%v", errNoS3ConfigForBackup, err)
	}
}

func TestNewBackupManagerWithoutABSCreds(t *testing.T) {
	cfg := Config{}
	cl := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "testing"},
		Spec: api.ClusterSpec{
			Backup: &api.BackupPolicy{
				StorageType: api.BackupStorageTypeABS,
				MaxBackups:  1,
			},
		},
	}
	_, err := newBackupManager(cfg, cl, nil)
	if err != errNoABSCredsForBackup {
		t.Errorf("expect err=%v, get=%v", errNoABSCredsForBackup, err)
	}
}
