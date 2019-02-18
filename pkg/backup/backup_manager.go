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
	"context"
	"crypto/tls"
	"fmt"
	"sort"
	"time"

	"github.com/coreos/etcd-operator/pkg/backup/writer"
	"github.com/coreos/etcd-operator/pkg/util/constants"

	"github.com/coreos/etcd/clientv3"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// BackupManager backups an etcd cluster.
type BackupManager struct {
	kubecli kubernetes.Interface

	endpoints     []string
	namespace     string
	etcdTLSConfig *tls.Config

	bw writer.Writer
}

// NewBackupManagerFromWriter creates a BackupManager with backup writer.
func NewBackupManagerFromWriter(kubecli kubernetes.Interface, bw writer.Writer, tc *tls.Config, endpoints []string, namespace string) *BackupManager {
	return &BackupManager{
		kubecli:       kubecli,
		endpoints:     endpoints,
		namespace:     namespace,
		etcdTLSConfig: tc,
		bw:            bw,
	}
}

// SaveSnap uses backup writer to save etcd snapshot to a specified S3 path
// and returns backup etcd server's kv store revision and its version.
func (bm *BackupManager) SaveSnap(ctx context.Context, s3Path string, isPeriodic bool) (int64, string, *metav1.Time, error) {
	now := time.Now().UTC()
	etcdcli, rev, err := bm.etcdClientWithMaxRevision(ctx)
	if err != nil {
		return 0, "", nil, fmt.Errorf("create etcd client failed: %v", err)
	}
	defer etcdcli.Close()

	resp, err := etcdcli.Status(ctx, etcdcli.Endpoints()[0])
	if err != nil {
		return 0, "", nil, fmt.Errorf("failed to retrieve etcd version from the status call: %v", err)
	}

	rc, err := etcdcli.Snapshot(ctx)
	if err != nil {
		return 0, "", nil, fmt.Errorf("failed to receive snapshot (%v)", err)
	}
	defer rc.Close()
	if isPeriodic {
		s3Path = fmt.Sprintf(s3Path+"_v%d_%s", rev, now.Format("2006-01-02-15:04:05"))
	}
	_, err = bm.bw.Write(ctx, s3Path, rc)
	if err != nil {
		return 0, "", nil, fmt.Errorf("failed to write snapshot (%v)", err)
	}
	return rev, resp.Version, &metav1.Time{Time: now}, nil
}

// EnsureMaxBackup to ensure the number of snapshot is under maxcount
// if the number of snapshot exceeded than maxcount, delete oldest snapshot
func (bm *BackupManager) EnsureMaxBackup(ctx context.Context, basePath string, maxCount int) error {
	savedSnapShots, err := bm.bw.List(ctx, basePath)
	if err != nil {
		return fmt.Errorf("failed to get exisiting snapshots: %v", err)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(savedSnapShots)))
	for i, snapshotPath := range savedSnapShots {
		if i < maxCount {
			continue
		}
		err := bm.bw.Delete(ctx, snapshotPath)
		if err != nil {
			return fmt.Errorf("failed to delete snapshot: %v", err)
		}
	}
	return nil
}

// etcdClientWithMaxRevision gets the etcd endpoint with the maximum kv store revision
// and returns the etcd client of that member.
func (bm *BackupManager) etcdClientWithMaxRevision(ctx context.Context) (*clientv3.Client, int64, error) {
	etcdcli, rev, err := getClientWithMaxRev(ctx, bm.endpoints, bm.etcdTLSConfig)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get etcd client with maximum kv store revision: %v", err)
	}
	return etcdcli, rev, nil
}

func getClientWithMaxRev(ctx context.Context, endpoints []string, tc *tls.Config) (*clientv3.Client, int64, error) {
	mapEps := make(map[string]*clientv3.Client)
	var maxClient *clientv3.Client
	maxRev := int64(0)
	errors := make([]string, 0)
	for _, endpoint := range endpoints {
		// TODO: update clientv3 to 3.2.x and then use ctx as in clientv3.Config.
		cfg := clientv3.Config{
			Endpoints:   []string{endpoint},
			DialTimeout: constants.DefaultDialTimeout,
			TLS:         tc,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to create etcd client for endpoint (%v): %v", endpoint, err))
			continue
		}
		mapEps[endpoint] = etcdcli

		resp, err := etcdcli.Get(ctx, "/", clientv3.WithSerializable())
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to get revision from endpoint (%s)", endpoint))
			continue
		}

		logrus.Infof("getMaxRev: endpoint %s revision (%d)", endpoint, resp.Header.Revision)
		if resp.Header.Revision > maxRev {
			maxRev = resp.Header.Revision
			maxClient = etcdcli
		}
	}

	// close all open clients that are not maxClient.
	for _, cli := range mapEps {
		if cli == maxClient {
			continue
		}
		cli.Close()
	}

	if maxClient == nil {
		return nil, 0, fmt.Errorf("could not create an etcd client for the max revision purpose from given endpoints (%v)", endpoints)
	}

	var err error
	if len(errors) > 0 {
		errorStr := ""
		for _, errStr := range errors {
			errorStr += errStr + "\n"
		}
		err = fmt.Errorf(errorStr)
	}

	return maxClient, maxRev, err
}
