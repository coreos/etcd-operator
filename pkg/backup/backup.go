// Copyright 2016 The kube-etcd-controller Authors
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
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/kube-etcd-controller/pkg/spec"
	"github.com/coreos/kube-etcd-controller/pkg/util/constants"
	"github.com/coreos/kube-etcd-controller/pkg/util/etcdutil"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

const (
	backupTmpDir         = "tmp"
	backupFilePerm       = 0600
	backupFilenameSuffix = "etcd.backup"
)

type Backup struct {
	kclient *unversioned.Client

	clusterName string
	namespace   string
	policy      spec.BackupPolicy
	listenAddr  string

	backupNow chan chan error
}

func New(kclient *unversioned.Client, clusterName, ns string, policy spec.BackupPolicy, listenAddr string) *Backup {
	return &Backup{
		kclient:     kclient,
		clusterName: clusterName,
		namespace:   ns,
		policy:      policy,
		listenAddr:  listenAddr,

		backupNow: make(chan chan error),
	}
}

func (b *Backup) Run() {
	// We created not only backup dir and but also tmp dir under it.
	// tmp dir is used to store intermediate snapshot files.
	// It will be no-op if target dir existed.
	if err := os.MkdirAll(filepath.Join(constants.BackupDir, backupTmpDir), 0700); err != nil {
		panic(err)
	}

	go b.startHTTP()

	lastSnapRev := int64(0)
	interval := constants.DefaultSnapshotInterval
	if b.policy.SnapshotIntervalInSecond != 0 {
		interval = time.Duration(b.policy.SnapshotIntervalInSecond) * time.Second
	}
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
	pods, err := b.kclient.Pods(b.namespace).List(k8sutil.EtcdPodListOpt(b.clusterName))
	if err != nil {
		return lastSnapRev, err
	}
	if len(pods.Items) == 0 {
		msg := "no running etcd pods found"
		logrus.Warning(msg)
		return lastSnapRev, fmt.Errorf(msg)
	}
	member, rev, err := getMemberWithMaxRev(pods)
	if err != nil {
		return lastSnapRev, err
	}
	if member == nil {
		logrus.Warning("no reachable member")
		return lastSnapRev, fmt.Errorf("no reachable member")
	}
	if rev == lastSnapRev {
		logrus.Info("skipped creating new backup: no change since last time")
		return lastSnapRev, nil
	}

	log.Printf("saving backup for cluster (%s)", b.clusterName)
	if err := writeSnap(member, constants.BackupDir, rev); err != nil {
		err = fmt.Errorf("write snapshot failed: %v", err)
		return lastSnapRev, err
	}
	return rev, nil
}

func writeSnap(m *etcdutil.Member, backupDir string, rev int64) error {
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

	rc, err := etcdcli.Maintenance.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to receive snapshot (%v)", err)
	}
	defer rc.Close()

	filename := makeFilename(rev)
	tmpfile, err := os.OpenFile(filepath.Join(backupDir, backupTmpDir, filename), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, backupFilePerm)
	if err != nil {
		return fmt.Errorf("failed to create snapshot tempfile: %v", err)
	}
	n, err := io.Copy(tmpfile, rc)
	if err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		return fmt.Errorf("failed to save snapshot: %v", err)
	}
	cancel()
	tmpfile.Close()

	nextSnapshotName := filepath.Join(backupDir, filename)
	err = os.Rename(tmpfile.Name(), nextSnapshotName)
	if err != nil {
		os.Remove(tmpfile.Name())
		return fmt.Errorf("rename snapshot from %s to %s failed: %v", tmpfile.Name(), nextSnapshotName, err)
	}
	log.Printf("saved snapshot %s (size: %d) successfully", nextSnapshotName, n)
	return nil
}

func makeFilename(rev int64) string {
	// TODO: version aware backup.
	return fmt.Sprintf("%s.%016x.%s", "etcd-version", rev, backupFilenameSuffix)
}

func getMemberWithMaxRev(pods *api.PodList) (*etcdutil.Member, int64, error) {
	var member *etcdutil.Member
	maxRev := int64(0)
	for i := range pods.Items {
		m := &etcdutil.Member{Name: pods.Items[i].Name}
		cfg := clientv3.Config{
			Endpoints:   []string{m.ClientAddr()},
			DialTimeout: constants.DefaultDialTimeout,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create etcd client (%v)", err)
		}
		defer etcdcli.Close()
		ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
		resp, err := etcdcli.Get(ctx, "/", clientv3.WithSerializable())
		if err != nil {
			return nil, 0, fmt.Errorf("etcdcli.Get failed: %v", err)
		}
		logrus.Infof("member: %s, revision: %d", m.Name, resp.Header.Revision)
		if resp.Header.Revision > maxRev {
			maxRev = resp.Header.Revision
			member = m
		}
	}
	return member, maxRev, nil
}
