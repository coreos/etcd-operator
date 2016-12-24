// Copyright 2016 The etcd-operator Authors
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
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd-operator/pkg/cluster/backupstorage"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
)

type backupManager struct {
	logger *logrus.Entry

	config      Config
	etcdCluster *spec.EtcdCluster
	s           backupstorage.Storage
}

func newBackupManager(c Config, e *spec.EtcdCluster, l *logrus.Entry, isNewCluster bool) (*backupManager, error) {
	bm := &backupManager{
		config:      c,
		etcdCluster: e,
		logger:      l,
	}
	hasExist := false
	if !isNewCluster {
		hasExist = true
	} else if r := e.Spec.Restore; r != nil && r.BackupClusterName == e.Name {
		hasExist = true // we will reuse the storage to restore cluster
	}
	var err error
	bm.s, err = bm.setupStorage(hasExist)
	if err != nil {
		return nil, err
	}
	return bm, nil
}

func (bm *backupManager) setup() error {
	if r := bm.etcdCluster.Spec.Restore; r != nil {
		bm.logger.Infof("restoring cluster from existing backup (%s)", r.BackupClusterName)
		if bm.etcdCluster.Name != r.BackupClusterName {
			if err := bm.s.Clone(r.BackupClusterName); err != nil {
				return err
			}
		}
	}

	return bm.runSidecar()
}

func (bm *backupManager) setupStorage(hasExist bool) (s backupstorage.Storage, err error) {
	e, c := bm.etcdCluster, bm.config

	b := e.Spec.Backup
	switch b.StorageType {
	case spec.BackupStorageTypePersistentVolume, spec.BackupStorageTypeDefault:
		s, err = backupstorage.NewPVStorage(c.KubeCli, e.Name, e.Namespace, c.PVProvisioner, *b, hasExist)
	case spec.BackupStorageTypeS3:
		s, err = backupstorage.NewS3Storage(c.S3Context, c.KubeCli, e.Name, e.Namespace, *b, hasExist)
	}
	return s, err
}

func (bm *backupManager) runSidecar() error {
	e, c := bm.etcdCluster, bm.config
	podSpec, err := k8sutil.MakeBackupPodSpec(e.Name, e.Spec.Backup)
	if err != nil {
		return err
	}
	switch e.Spec.Backup.StorageType {
	case spec.BackupStorageTypeDefault, spec.BackupStorageTypePersistentVolume:
		podSpec = k8sutil.PodSpecWithPV(podSpec, e.Name)
	case spec.BackupStorageTypeS3:
		podSpec = k8sutil.PodSpecWithS3(podSpec, c.S3Context)
	}
	err = k8sutil.CreateBackupReplicaSetAndService(c.KubeCli, e.Name, e.Namespace, *podSpec)
	if err != nil {
		return fmt.Errorf("failed to create backup replica set and service: %v", err)
	}
	bm.logger.Info("backup replica set and service created")
	return nil
}

func (bm *backupManager) cleanup() error {
	e, c := bm.etcdCluster, bm.config
	err := k8sutil.DeleteBackupReplicaSetAndService(c.KubeCli, e.Name, e.Namespace)
	if err != nil {
		return fmt.Errorf("fail to delete backup ReplicaSet and Service: %v", err)
	}
	bm.logger.Infof("backup replica set and service deleted")

	err = bm.s.Delete()
	if err != nil {
		return fmt.Errorf("fail to delete store: %v", err)
	}
	return nil
}
