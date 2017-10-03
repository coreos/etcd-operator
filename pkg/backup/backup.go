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
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/backup/env"
	"github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/util/backuputil"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"golang.org/x/net/context"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	PVBackupV1 = "v1"

	maxRecentBackupStatusCount = 10
)

type Backup struct {
	kclient kubernetes.Interface

	clusterName   string
	namespace     string
	policy        api.BackupPolicy
	listenAddr    string
	etcdTLSConfig *tls.Config
	selfHosted    bool

	be backend

	backupNow chan chan backupNowAck

	// recentBackupStatus keeps the statuses of 'maxRecentBackupStatusCount' recent backups.
	recentBackupsStatus []backupapi.BackupStatus
}

func New(kclient kubernetes.Interface, clusterName, ns string, sp api.ClusterSpec, listenAddr string) (*Backup, error) {
	bdir := path.Join(constants.BackupMountDir, PVBackupV1, clusterName)
	// We created not only backup dir and but also tmp dir under it.
	// tmp dir is used to store intermediate snapshot files.
	// It will be no-op if target dir existed.
	tmpDir := path.Join(bdir, backupTmpDir)
	err := os.MkdirAll(tmpDir, 0700)
	if err != nil {
		return nil, err
	}

	var be backend
	switch sp.Backup.StorageType {
	case api.BackupStorageTypePersistentVolume, api.BackupStorageTypeDefault:
		be = &fileBackend{dir: bdir}
	case api.BackupStorageTypeS3:
		s3Prefix := ""
		if sp.Backup.S3 != nil {
			s3Prefix = sp.Backup.S3.Prefix
		}
		s3cli, err := s3.New(os.Getenv(env.AWSS3Bucket), backupapi.ToS3Prefix(s3Prefix, ns, clusterName))
		if err != nil {
			return nil, err
		}

		be = &s3Backend{
			dir: tmpDir,
			S3:  s3cli,
		}
	case api.BackupStorageTypeABS:
		absCli, err := abs.New(os.Getenv(env.ABSContainer),
			os.Getenv(env.ABSStorageAccount),
			os.Getenv(env.ABSStorageKey),
			path.Join(ns, clusterName))
		if err != nil {
			return nil, err
		}

		be = &absBackend{
			ABS: absCli,
		}
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

	return &Backup{
		kclient:       kclient,
		clusterName:   clusterName,
		namespace:     ns,
		policy:        *sp.Backup,
		listenAddr:    listenAddr,
		be:            be,
		etcdTLSConfig: tc,
		selfHosted:    sp.SelfHosted != nil,

		backupNow: make(chan chan backupNowAck),
	}, nil
}

func (b *Backup) Run() {
	go b.startHTTP()

	lastSnapRev := b.getLatestBackupRev()
	interval := constants.DefaultSnapshotInterval
	if b.policy.BackupIntervalInSecond != 0 {
		interval = time.Duration(b.policy.BackupIntervalInSecond) * time.Second
	}

	go func() {
		if b.policy.MaxBackups == 0 {
			return
		}
		for {
			<-time.After(10 * time.Second)
			err := b.be.purge(b.policy.MaxBackups)
			if err != nil {
				logrus.Errorf("fail to purge backups: %v", err)
			}
		}
	}()

	for {
		var ackchan chan backupNowAck
		select {
		case <-time.After(interval):
		case ackchan = <-b.backupNow:
			logrus.Info("received a backup request")
		}

		rev, err := b.saveSnap(lastSnapRev)
		if err != nil {
			logrus.Errorf("failed to save snapshot: %v", err)
		}
		lastSnapRev = rev

		if ackchan != nil {
			ack := backupNowAck{err: err}
			if err == nil {
				ack.status = b.getLatestBackupStatus()
			}
			ackchan <- ack
		}
	}
}

func (b *Backup) saveSnap(lastSnapRev int64) (int64, error) {
	podList, err := b.kclient.Core().Pods(b.namespace).List(k8sutil.ClusterListOpt(b.clusterName))
	if err != nil {
		return lastSnapRev, err
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
		return lastSnapRev, fmt.Errorf(msg)
	}
	member, rev := backuputil.GetMemberWithMaxRev(pods, b.etcdTLSConfig)
	if member == nil {
		logrus.Warning("no reachable member")
		return lastSnapRev, fmt.Errorf("no reachable member")
	}

	if rev <= lastSnapRev {
		logrus.Info("skipped creating new backup: no change since last time")
		return lastSnapRev, nil
	}

	log.Printf("saving backup for cluster (%s)", b.clusterName)
	if err := b.writeSnap(member, rev); err != nil {
		err = fmt.Errorf("write snapshot failed: %v", err)
		return lastSnapRev, err
	}
	return rev, nil
}

func (b *Backup) writeSnap(m *etcdutil.Member, rev int64) error {
	start := time.Now()

	cfg := clientv3.Config{
		Endpoints:   []string{m.ClientURL()},
		DialTimeout: constants.DefaultDialTimeout,
		TLS:         b.etcdTLSConfig,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create etcd client (%v)", err)
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.Maintenance.Status(ctx, m.ClientURL())
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultSnapshotTimeout)
	rc, err := etcdcli.Maintenance.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to receive snapshot (%v)", err)
	}
	defer cancel()
	defer rc.Close()

	n, err := b.be.save(resp.Version, rev, rc)
	if err != nil {
		return err
	}

	bs := backupapi.BackupStatus{
		CreationTime:     time.Now().Format(time.RFC3339),
		Size:             toMB(n),
		Version:          resp.Version,
		Revision:         rev,
		TimeTookInSecond: int(time.Since(start).Seconds() + 1),
	}
	b.recentBackupsStatus = append(b.recentBackupsStatus, bs)
	if len(b.recentBackupsStatus) > maxRecentBackupStatusCount {
		b.recentBackupsStatus = b.recentBackupsStatus[1:]
	}

	return nil
}

func (b *Backup) getLatestBackupRev() int64 {
	// If there is any error, we just exit backup sidecar because we can't serve the backup any way.
	name, err := b.be.getLatest()
	if err != nil {
		logrus.Fatal(err)
	}
	if len(name) == 0 {
		return 0
	}
	rev, err := getRev(name)
	if err != nil {
		logrus.Fatal(err)
	}
	return rev
}

func (b *Backup) getLatestBackupStatus() backupapi.BackupStatus {
	return b.recentBackupsStatus[len(b.recentBackupsStatus)-1]
}
