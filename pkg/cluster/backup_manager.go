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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/etcd-operator/client/experimentalclient"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/cluster/backupstorage"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	defaultBackupHTTPTimeout     = 5 * time.Second
	defaultBackupCreatingTimeout = 1 * time.Minute
)

var (
	errNoPVForBackup       = errors.New("no backup could be created due to PVProvisioner (none) is set")
	errNoS3ConfigForBackup = errors.New("no backup could be created due to S3 configuration not set")
)

type backupManager struct {
	logger *logrus.Entry

	config  Config
	cluster *spec.Cluster
	s       backupstorage.Storage

	bc experimentalclient.Backup
}

func newBackupManager(c Config, cl *spec.Cluster, l *logrus.Entry) (*backupManager, error) {
	bm := &backupManager{
		config:  c,
		cluster: cl,
		logger:  l,
		bc:      experimentalclient.NewBackup(&http.Client{}, "http", cl.Metadata.GetName()),
	}
	var err error
	bm.s, err = bm.setupStorage()
	if err != nil {
		return nil, err
	}
	return bm, nil
}

// setupStorage will only set up the necessary structs in order for backup manager to
// use the storage. It doesn't creates the actual storage here.
// We also need to check some restore logic in setup().
func (bm *backupManager) setupStorage() (s backupstorage.Storage, err error) {
	cl, c := bm.cluster, bm.config

	b := cl.Spec.Backup
	switch b.StorageType {
	case spec.BackupStorageTypePersistentVolume, spec.BackupStorageTypeDefault:
		if c.PVProvisioner == constants.PVProvisionerNone {
			return nil, errNoPVForBackup
		}
		s, err = backupstorage.NewPVStorage(c.KubeCli, cl.Metadata.Name, cl.Metadata.Namespace, c.PVProvisioner, *b)
	case spec.BackupStorageTypeS3:
		if len(c.S3Context.AWSConfig) == 0 {
			return nil, errNoS3ConfigForBackup
		}
		s, err = backupstorage.NewS3Storage(c.S3Context, c.KubeCli, cl.Metadata.Name, cl.Metadata.Namespace, *b)
	}
	return s, err
}

func (bm *backupManager) setup() error {
	r := bm.cluster.Spec.Restore
	restoreSameNameCluster := r != nil && r.BackupClusterName == bm.cluster.Metadata.Name

	// There is only one case that we don't need to create underlying storage.
	// That is, the storage already exists and we are restoring cluster from it.
	if !restoreSameNameCluster {
		if err := bm.s.Create(); err != nil {
			return err
		}
	}

	if r != nil {
		bm.logger.Infof("restoring cluster from existing backup (%s)", r.BackupClusterName)
		if bm.cluster.Metadata.Name != r.BackupClusterName {
			if err := bm.s.Clone(r.BackupClusterName); err != nil {
				return err
			}
		}
	}

	return bm.runSidecar()
}

func (bm *backupManager) runSidecar() error {
	cl, c := bm.cluster, bm.config
	podSpec, err := k8sutil.NewBackupPodSpec(cl.Metadata.Name, bm.config.ServiceAccount, cl.Spec)
	if err != nil {
		return err
	}
	switch cl.Spec.Backup.StorageType {
	case spec.BackupStorageTypeDefault, spec.BackupStorageTypePersistentVolume:
		podSpec = k8sutil.PodSpecWithPV(podSpec, cl.Metadata.Name)
	case spec.BackupStorageTypeS3:
		podSpec = k8sutil.PodSpecWithS3(podSpec, c.S3Context)
	}
	if err = bm.createBackupReplicaSet(*podSpec); err != nil {
		return fmt.Errorf("failed to create backup replica set: %v", err)
	}
	if err = bm.createBackupService(); err != nil {
		return fmt.Errorf("failed to create backup service: %v", err)
	}
	bm.logger.Info("backup replica set and service created")
	return nil
}

func (bm *backupManager) createBackupReplicaSet(podSpec v1.PodSpec) error {
	rs := k8sutil.NewBackupReplicaSetManifest(bm.cluster.Metadata.Name, podSpec, bm.cluster.AsOwner())
	_, err := bm.config.KubeCli.ExtensionsV1beta1().ReplicaSets(bm.cluster.Metadata.Namespace).Create(rs)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	return nil
}

func (bm *backupManager) createBackupService() error {
	svc := k8sutil.NewBackupServiceManifest(bm.cluster.Metadata.Name, bm.cluster.AsOwner())
	_, err := bm.config.KubeCli.CoreV1().Services(bm.cluster.Metadata.Namespace).Create(svc)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	return nil
}

func (bm *backupManager) cleanup() error {
	// Only need to delete storage, Kubernetes related resources will be deleted by the GC.
	err := bm.s.Delete()
	if err != nil {
		return fmt.Errorf("fail to delete backup storage: %v", err)
	}
	return nil
}

func (bm *backupManager) requestBackup() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultBackupHTTPTimeout+defaultBackupCreatingTimeout)
	defer cancel()
	return bm.bc.Request(ctx)
}

func (bm *backupManager) checkBackupExist(ver string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultBackupHTTPTimeout)
	defer cancel()
	return bm.bc.Exist(ctx, ver)
}

func (bm *backupManager) getStatus() (*backupapi.ServiceStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultBackupHTTPTimeout)
	defer cancel()
	return bm.bc.ServiceStatus(ctx)
}

func backupServiceStatusToTPRBackupServiceStatu(s *backupapi.ServiceStatus) *spec.BackupServiceStatus {
	b, err := json.Marshal(s)
	if err != nil {
		panic("unexpected json error")
	}

	var bs spec.BackupServiceStatus
	err = json.Unmarshal(b, &bs)
	if err != nil {
		panic("unexpected json error")
	}
	return &bs
}
