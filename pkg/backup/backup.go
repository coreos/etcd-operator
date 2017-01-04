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
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/etcd-operator/pkg/backup/env"
	"github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coreos/etcd/clientv3"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type Backup struct {
	kclient *unversioned.Client

	clusterName string
	namespace   string
	policy      spec.BackupPolicy
	listenAddr  string

	be backend

	backupNow chan chan error
}

func New(kclient *unversioned.Client, clusterName, ns string, policy spec.BackupPolicy, listenAddr string) *Backup {
	// We created not only backup dir and but also tmp dir under it.
	// tmp dir is used to store intermediate snapshot files.
	// It will be no-op if target dir existed.
	if err := os.MkdirAll(filepath.Join(constants.BackupDir, backupTmpDir), 0700); err != nil {
		panic(err)
	}

	var be backend
	switch policy.StorageType {
	case spec.BackupStorageTypePersistentVolume, spec.BackupStorageTypeDefault:
		be = &fileBackend{dir: constants.BackupDir}
	case spec.BackupStorageTypeS3:
		prefix := clusterName + "/"
		s3cli, err := s3.New(os.Getenv(env.AWSS3Bucket), prefix, session.Options{
			SharedConfigState: session.SharedConfigEnable,
		})
		if err != nil {
			panic(err)
		}
		be = &s3Backend{
			dir: constants.BackupDir,
			S3:  s3cli,
		}
	default:
		logrus.Fatalf("unsupported storage type: %v", policy.StorageType)
	}

	return &Backup{
		kclient:     kclient,
		clusterName: clusterName,
		namespace:   ns,
		policy:      policy,
		listenAddr:  listenAddr,
		be:          be,

		backupNow: make(chan chan error),
	}
}

func (b *Backup) Run() {
	go b.startHTTP()

	lastSnapRev := int64(0)
	interval := constants.DefaultSnapshotInterval
	if b.policy.BackupIntervalInSecond != 0 {
		interval = time.Duration(b.policy.BackupIntervalInSecond) * time.Second
	}

	go func() {
		for {
			<-time.After(10 * time.Second)
			err := b.be.purge(b.policy.MaxBackups)
			if err != nil {
				logrus.Errorf("fail to purge backups: %v", err)
			}
		}
	}()

	for {
		var ackchan chan error
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
			ackchan <- err
		}
	}
}

func (b *Backup) saveSnap(lastSnapRev int64) (int64, error) {
	podList, err := b.kclient.Pods(b.namespace).List(k8sutil.ClusterListOpt(b.clusterName))
	if err != nil {
		return lastSnapRev, err
	}

	var pods []*api.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == api.PodRunning {
			pods = append(pods, pod)
		}
	}

	if len(pods) == 0 {
		msg := "no running etcd pods found"
		logrus.Warning(msg)
		return lastSnapRev, fmt.Errorf(msg)
	}
	member, rev := getMemberWithMaxRev(pods)
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
	cfg := clientv3.Config{
		Endpoints:   []string{m.ClientAddr()},
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create etcd client (%v)", err)
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.Maintenance.Status(ctx, m.ClientAddr())
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	rc, err := etcdcli.Maintenance.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to receive snapshot (%v)", err)
	}
	defer cancel()
	defer rc.Close()

	return b.be.save(resp.Version, rev, rc)
}

func getMemberWithMaxRev(pods []*api.Pod) (*etcdutil.Member, int64) {
	var member *etcdutil.Member
	maxRev := int64(0)
	for _, pod := range pods {
		m := &etcdutil.Member{Name: pod.Name}
		cfg := clientv3.Config{
			Endpoints:   []string{m.ClientAddr()},
			DialTimeout: constants.DefaultDialTimeout,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			logrus.Fatalf("failed to create etcd client (%v)", err)
		}
		defer etcdcli.Close()

		ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
		resp, err := etcdcli.Get(ctx, "/", clientv3.WithSerializable())
		if err != nil {
			logrus.Warningf("getMaxRev: failed to get revision from member %s (%s)", m.Name, m.ClientAddr())
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
