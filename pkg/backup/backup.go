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
	"log"
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

	"github.com/coreos/etcd/clientv3"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	PVBackupV1 = "v1"

	maxRecentBackupStatusCount = 10
)

// BackupManager backups an etcd cluster.
type BackupManager struct {
	kclient kubernetes.Interface

	clusterName   string
	namespace     string
	etcdTLSConfig *tls.Config

	be backend.Backend
}

// BackupController controls when to do backup based on backup policy and incoming HTTP backup requests.
type BackupController struct {
	listenAddr    string
	backupNow     chan chan backupNowAck
	policy        api.BackupPolicy
	backupManager *BackupManager
	// recentBackupStatus keeps the statuses of 'maxRecentBackupStatusCount' recent backups.
	recentBackupsStatus []backupapi.BackupStatus
}

// NewBackupController creates a BackupController.
func NewBackupController(kclient kubernetes.Interface, clusterName, ns string, sp api.ClusterSpec, listenAddr string) (*BackupController, error) {
	bdir := path.Join(constants.BackupMountDir, PVBackupV1, clusterName)
	// We created not only backup dir and but also tmp dir under it.
	// tmp dir is used to store intermediate snapshot files.
	// It will be no-op if target dir existed.
	tmpDir := path.Join(bdir, util.BackupTmpDir)
	err := os.MkdirAll(tmpDir, 0700)
	if err != nil {
		return nil, err
	}

	var be backend.Backend
	switch sp.Backup.StorageType {
	case api.BackupStorageTypePersistentVolume, api.BackupStorageTypeDefault:
		be = backend.NewFileBackend(bdir)
	case api.BackupStorageTypeS3:
		s3Prefix := ""
		if sp.Backup.S3 != nil {
			s3Prefix = sp.Backup.S3.Prefix
		}
		s3cli, err := s3.New(os.Getenv(env.AWSS3Bucket), backupapi.ToS3Prefix(s3Prefix, ns, clusterName))
		if err != nil {
			return nil, err
		}
		be = backend.NewS3Backend(s3cli, tmpDir)
	case api.BackupStorageTypeABS:
		absCli, err := abs.New(os.Getenv(env.ABSContainer),
			os.Getenv(env.ABSStorageAccount),
			os.Getenv(env.ABSStorageKey),
			path.Join(ns, clusterName))
		if err != nil {
			return nil, err
		}
		be = backend.NewAbsBackend(absCli)
	default:
		return nil, fmt.Errorf("unsupported storage type: %v", sp.Backup.StorageType)
	}

	var tc *tls.Config
	if sp.TLS.IsSecureClient() {
		d, err := k8sutil.GetTLSDataFromSecret(kclient, ns, sp.TLS.Static.OperatorSecret)
		if err != nil {
			return nil, err
		}
		tc, err = etcdutil.NewTLSConfig(d.CertData, d.KeyData, d.CAData)
		if err != nil {
			return nil, err
		}
	}

	bm := &BackupManager{
		kclient:       kclient,
		clusterName:   clusterName,
		namespace:     ns,
		be:            be,
		etcdTLSConfig: tc,
	}

	return &BackupController{
		listenAddr:    listenAddr,
		backupNow:     make(chan chan backupNowAck),
		policy:        *sp.Backup,
		backupManager: bm,
	}, nil
}

// Run starts BackupController controller where it
// controlls backups based on backup policy and HTTP backup requests.
func (bc *BackupController) Run() {
	go bc.startHTTP()

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
			if err == nil {
				ack.status = bc.recentBackupsStatus[len(bc.recentBackupsStatus)-1]
			}
			ackchan <- ack
		}
	}
}

// SaveSnap saves the snapshot if snapshot's revision is greater than the given lastSnapRev
// and returns a BackupStatus containing saving backup metadata if SaveSnap succeeds.
func (bm *BackupManager) SaveSnap(lastSnapRev int64) (*backupapi.BackupStatus, error) {
	podList, err := bm.kclient.Core().Pods(bm.namespace).List(k8sutil.ClusterListOpt(bm.clusterName))
	if err != nil {
		return nil, err
	}

	var pods []*v1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == v1.PodRunning {
			pods = append(pods, pod)
		}
	}

	if len(pods) == 0 {
		msg := "no running etcd pods found"
		logrus.Warning(msg)
		return nil, fmt.Errorf(msg)
	}
	member, rev := getMemberWithMaxRev(pods, bm.etcdTLSConfig)
	if member == nil {
		logrus.Warning("no reachable member")
		return nil, fmt.Errorf("no reachable member")
	}

	if rev <= lastSnapRev {
		logrus.Info("skipped creating new backup: no change since last time")
		return nil, nil
	}

	log.Printf("saving backup for cluster (%s)", bm.clusterName)
	bs, err := bm.writeSnap(member, rev)
	if err != nil {
		err = fmt.Errorf("write snapshot failed: %v", err)
		return nil, err
	}
	return bs, nil
}

func (bm *BackupManager) writeSnap(m *etcdutil.Member, rev int64) (*backupapi.BackupStatus, error) {
	start := time.Now()

	cfg := clientv3.Config{
		Endpoints:   []string{m.ClientURL()},
		DialTimeout: constants.DefaultDialTimeout,
		TLS:         bm.etcdTLSConfig,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client (%v)", err)
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.Maintenance.Status(ctx, m.ClientURL())
	cancel()
	if err != nil {
		return nil, err
	}

	ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultSnapshotTimeout)
	rc, err := etcdcli.Maintenance.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to receive snapshot (%v)", err)
	}
	defer cancel()
	defer rc.Close()

	n, err := bm.be.Save(resp.Version, rev, rc)
	if err != nil {
		return nil, err
	}

	bs := &backupapi.BackupStatus{
		CreationTime:     time.Now().Format(time.RFC3339),
		Size:             util.ToMB(n),
		Version:          resp.Version,
		Revision:         rev,
		TimeTookInSecond: int(time.Since(start).Seconds() + 1),
	}

	return bs, nil
}

func getMemberWithMaxRev(pods []*v1.Pod, tc *tls.Config) (*etcdutil.Member, int64) {
	var member *etcdutil.Member
	maxRev := int64(0)
	for _, pod := range pods {
		m := &etcdutil.Member{
			Name:         pod.Name,
			Namespace:    pod.Namespace,
			SecureClient: tc != nil,
		}
		cfg := clientv3.Config{
			Endpoints:   []string{m.ClientURL()},
			DialTimeout: constants.DefaultDialTimeout,
			TLS:         tc,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			logrus.Warningf("failed to create etcd client for pod (%v): %v", pod.Name, err)
			continue
		}
		defer etcdcli.Close()

		ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
		resp, err := etcdcli.Get(ctx, "/", clientv3.WithSerializable())
		cancel()
		if err != nil {
			logrus.Warningf("getMaxRev: failed to get revision from member %s (%s)", m.Name, m.ClientURL())
			continue
		}

		logrus.Infof("getMaxRev: member %s revision (%d)", m.Name, resp.Header.Revision)
		if resp.Header.Revision > maxRev {
			maxRev = resp.Header.Revision
			member = m
		}
	}
	return member, maxRev
}

func (b *BackupManager) getLatestBackupRev() int64 {
	// If there is any error, we just exit backup sidecar because we can't serve the backup any way.
	name, err := b.be.GetLatest()
	if err != nil {
		logrus.Fatal(err)
	}
	if len(name) == 0 {
		return 0
	}
	rev, err := util.GetRev(name)
	if err != nil {
		logrus.Fatal(err)
	}
	return rev
}
