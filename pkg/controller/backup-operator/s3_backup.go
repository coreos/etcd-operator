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

package controller

import (
	"context"
	"crypto/tls"
	"fmt"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup"
	"github.com/coreos/etcd-operator/pkg/backup/writer"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"

	"k8s.io/client-go/kubernetes"
)

// TODO: replace this with generic backend interface for other options (PV, Azure)
// handleS3 saves etcd cluster's backup to specificed S3 path.
func handleS3(ctx context.Context, kubecli kubernetes.Interface, s *api.S3BackupSource, endpoints []string, clientTLSSecret,
	namespace string, isPeriodic bool, maxBackup int) (*api.BackupStatus, error) {
	// TODO: controls NewClientFromSecret with ctx. This depends on upstream kubernetes to support API calls with ctx.
	cli, err := s3factory.NewClientFromSecret(kubecli, namespace, s.Endpoint, s.AWSSecret, s.ForcePathStyle)
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	var tlsConfig *tls.Config
	if tlsConfig, err = generateTLSConfig(kubecli, clientTLSSecret, namespace); err != nil {
		return nil, err
	}
	bm := backup.NewBackupManagerFromWriter(kubecli, writer.NewS3Writer(cli.S3), tlsConfig, endpoints, namespace)

	rev, etcdVersion, now, err := bm.SaveSnap(ctx, s.Path, isPeriodic)
	if err != nil {
		return nil, fmt.Errorf("failed to save snapshot (%v)", err)
	}
	if maxBackup > 0 {
		err := bm.EnsureMaxBackup(ctx, s.Path, maxBackup)
		if err != nil {
			return nil, fmt.Errorf("succeeded in saving snapshot but failed to delete old snapshot (%v)", err)
		}
	}
	return &api.BackupStatus{EtcdVersion: etcdVersion, EtcdRevision: rev, LastSuccessDate: *now}, nil
}
