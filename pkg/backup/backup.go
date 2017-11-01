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

package backup

import (
	"crypto/tls"
	"fmt"
	"os"
	"path"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup/abs"
	"github.com/coreos/etcd-operator/pkg/backup/backend"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/backup/env"
	"github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/backup/util"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

const (
	PVBackupV1 = "v1"

	maxRecentBackupStatusCount = 10
)

// BackupController controls when to do backup based on backup policy and incoming HTTP backup requests.
type BackupController struct {
	listenAddr    string
	backupNow     chan chan backupNowAck
	policy        api.BackupPolicy
	backupManager *BackupManager
	backupServer  *BackupServer
	// recentBackupStatus keeps the statuses of 'maxRecentBackupStatusCount' recent backups.
	recentBackupsStatus []backupapi.BackupStatus
}

// BackupControllerConfig contains configuration data to construct BackupController.
type BackupControllerConfig struct {
	Kubecli kubernetes.Interface

	ListenAddr  string
	ClusterName string
	Namespace   string

	TLS          *api.TLSPolicy
	BackupPolicy *api.BackupPolicy
}

// NewBackupController creates a BackupController.
func NewBackupController(config *BackupControllerConfig) (*BackupController, error) {
	var be backend.Backend
	bp := config.BackupPolicy

	switch bp.StorageType {
	case api.BackupStorageTypePersistentVolume, api.BackupStorageTypeDefault:
		bdir := path.Join(constants.BackupMountDir, PVBackupV1, config.ClusterName)
		err := os.MkdirAll(path.Join(bdir, util.BackupTmpDir), 0700)
		if err != nil {
			return nil, err
		}
		be = backend.NewFileBackend(bdir)
	case api.BackupStorageTypeS3:
		s3Prefix := ""
		if bp.S3 != nil {
			s3Prefix = bp.S3.Prefix
		}
		s3cli, err := s3.New(os.Getenv(env.AWSS3Bucket), backupapi.ToS3Prefix(s3Prefix, config.Namespace, config.ClusterName))
		if err != nil {
			return nil, err
		}
		be = backend.NewS3Backend(s3cli)
	case api.BackupStorageTypeABS:
		absCli, err := abs.New(os.Getenv(env.ABSContainer),
			os.Getenv(env.ABSStorageAccount),
			os.Getenv(env.ABSStorageKey),
			path.Join(config.Namespace, config.ClusterName))
		if err != nil {
			return nil, err
		}
		be = backend.NewAbsBackend(absCli)
	default:
		return nil, fmt.Errorf("unsupported storage type: %v", bp.StorageType)
	}

	var tc *tls.Config
	if config.TLS.IsSecureClient() {
		d, err := k8sutil.GetTLSDataFromSecret(config.Kubecli, config.Namespace, config.TLS.Static.OperatorSecret)
		if err != nil {
			return nil, err
		}
		tc, err = etcdutil.NewTLSConfig(d.CertData, d.KeyData, d.CAData)
		if err != nil {
			return nil, err
		}
	}

	bm := &BackupManager{
		kubecli:       config.Kubecli,
		clusterName:   config.ClusterName,
		namespace:     config.Namespace,
		be:            be,
		etcdTLSConfig: tc,
	}
	bs := &BackupServer{
		backend: be,
	}

	return &BackupController{
		listenAddr:    config.ListenAddr,
		backupNow:     make(chan chan backupNowAck),
		policy:        *bp,
		backupManager: bm,
		backupServer:  bs,
	}, nil
}

// Run starts BackupController controller where it
// controlls backups based on backup policy and HTTP backup requests.
func (bc *BackupController) Run() {
	lastSnapRev := bc.backupManager.getLatestBackupRev()
	interval := constants.DefaultSnapshotInterval
	if bc.policy.BackupIntervalInSecond != 0 {
		interval = time.Duration(bc.policy.BackupIntervalInSecond) * time.Second
	}

	go func() {
		if bc.policy.MaxBackups == 0 {
			return
		}
		for {
			<-time.After(10 * time.Second)
			err := bc.backupManager.be.Purge(bc.policy.MaxBackups)
			if err != nil {
				logrus.Errorf("fail to purge backups: %v", err)
			}
		}
	}()

	for {
		var ackchan chan backupNowAck
		select {
		case <-time.After(interval):
		case ackchan = <-bc.backupNow:
			logrus.Info("received a backup request")
		}

		bs, err := bc.backupManager.SaveSnap(lastSnapRev)
		if err != nil {
			logrus.Errorf("failed to save snapshot: %v", err)
		}

		if bs != nil {
			lastSnapRev = bs.Revision
			bc.recentBackupsStatus = append(bc.recentBackupsStatus, *bs)
			if len(bc.recentBackupsStatus) > maxRecentBackupStatusCount {
				bc.recentBackupsStatus = bc.recentBackupsStatus[1:]
			}
		}

		if ackchan != nil {
			ack := backupNowAck{err: err}
			if err == nil && len(bc.recentBackupsStatus) > 0 {
				ack.status = bc.recentBackupsStatus[len(bc.recentBackupsStatus)-1]
			}
			ackchan <- ack
		}
	}
}
