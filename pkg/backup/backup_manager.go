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
	"errors"
	"fmt"

	"github.com/coreos/etcd-operator/pkg/backup/writer"
	"github.com/coreos/etcd-operator/pkg/util/constants"

	"github.com/coreos/etcd/clientv3"
	"github.com/sirupsen/logrus"
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

// SaveSnap uses backup writer to save etcd snapshot to a specified S3 path.
func (bm *BackupManager) SaveSnap(s3Path string) error {
	etcdcli, _, err := bm.etcdClientWithMaxRevision()
	if err != nil {
		return fmt.Errorf("create etcd client failed: %v", err)
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultSnapshotTimeout)
	defer cancel() // Can't cancel() after Snapshot() because that will close the reader.
	rc, err := etcdcli.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("failed to receive snapshot (%v)", err)
	}
	defer rc.Close()

	_, err = bm.bw.Write(s3Path, rc)
	if err != nil {
		return fmt.Errorf("failed to write snapshot (%v)", err)
	}
	return nil
}

// etcdClientWithMaxRevision gets the etcd endpoint with the maximum kv store revision
// and returns the etcd client and the rev of that member.
func (bm *BackupManager) etcdClientWithMaxRevision() (*clientv3.Client, int64, error) {
	client, rev := getEndpointWithMaxRev(bm.endpoints, bm.etcdTLSConfig)
	if len(client) == 0 {
		return nil, 0, errors.New("non-reachable endpoints")
	}

	etcdcli, err := createEtcdClient(client, bm.etcdTLSConfig)
	if err != nil {
		return nil, 0, fmt.Errorf("create etcd client failed: %v", err)
	}
	return etcdcli, rev, nil
}

func getEndpointWithMaxRev(endpoints []string, tc *tls.Config) (string, int64) {
	var maxEndpoint string
	maxRev := int64(0)
	for _, endpoint := range endpoints {
		cfg := clientv3.Config{
			Endpoints:   []string{endpoint},
			DialTimeout: constants.DefaultDialTimeout,
			TLS:         tc,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			logrus.Warningf("failed to create etcd client for endpoint (%v): %v", err)
			continue
		}
		defer etcdcli.Close()

		ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
		resp, err := etcdcli.Get(ctx, "/", clientv3.WithSerializable())
		cancel()
		if err != nil {
			logrus.Warningf("getMaxRev: failed to get revision from endpoint (%s)", endpoint)
			continue
		}

		logrus.Infof("getMaxRev: endpoint %s revision (%d)", endpoint, resp.Header.Revision)
		if resp.Header.Revision > maxRev {
			maxRev = resp.Header.Revision
			maxEndpoint = endpoint
		}
	}
	return maxEndpoint, maxRev
}

func createEtcdClient(url string, tlsConfig *tls.Config) (*clientv3.Client, error) {
	cfg := clientv3.Config{
		Endpoints:   []string{url},
		DialTimeout: constants.DefaultDialTimeout,
		TLS:         tlsConfig,
	}
	return clientv3.New(cfg)
}
