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

package backup

import (
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/etcd-operator/pkg/backup/backend"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
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

// BackupManager backups an etcd cluster.
type BackupManager struct {
	kclient kubernetes.Interface

	clusterName   string
	namespace     string
	etcdTLSConfig *tls.Config

	be backend.Backend
}

// NewBackupManager creates a BackupManager.
func NewBackupManager(kclient kubernetes.Interface, clusterName string, namespace string, etcdTLSConfig *tls.Config, be backend.Backend) *BackupManager {
	return &BackupManager{
		kclient:       kclient,
		clusterName:   clusterName,
		namespace:     namespace,
		etcdTLSConfig: etcdTLSConfig,
		be:            be,
	}
}

// SaveSnap saves the latest snapshot if its revision is greater than the given lastSnapRev
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
		return nil, errors.New("no running etcd pods found")
	}
	member, rev := getMemberWithMaxRev(pods, bm.etcdTLSConfig)
	if member == nil {
		return nil, errors.New("no reachable member")
	}

	if rev <= lastSnapRev {
		logrus.Info("skipped creating new backup: no change since last time")
		return nil, nil
	}

	etcdcli, err := createEtcdClient(member.ClientURL(), bm.etcdTLSConfig)
	if err != nil {
		return nil, fmt.Errorf("create etcd client failed: %v", err)
	}
	defer etcdcli.Close()

	bs, err := bm.writeSnap(etcdcli.Maintenance, member.ClientURL(), rev)
	if err != nil {
		return nil, fmt.Errorf("write snapshot failed: %v", err)
	}
	logrus.Infof("saved backup (rev: %v, etcdVersion: %v) for cluster (%s)",
		bs.Revision, bs.Version, bm.clusterName)
	return bs, nil
}

func (bm *BackupManager) writeSnap(mcli clientv3.Maintenance, endpoint string, rev int64) (*backupapi.BackupStatus, error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := mcli.Status(ctx, endpoint)
	cancel()
	if err != nil {
		return nil, err
	}

	ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultSnapshotTimeout)
	rc, err := mcli.Snapshot(ctx)
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
	return util.MustParseRevision(name)
}

func createEtcdClient(url string, tlsConfig *tls.Config) (*clientv3.Client, error) {
	cfg := clientv3.Config{
		Endpoints:   []string{url},
		DialTimeout: constants.DefaultDialTimeout,
		TLS:         tlsConfig,
	}
	return clientv3.New(cfg)
}
