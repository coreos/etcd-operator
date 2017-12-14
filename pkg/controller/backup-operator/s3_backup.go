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
	"crypto/tls"
	"fmt"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup"
	"github.com/coreos/etcd-operator/pkg/backup/writer"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/client-go/kubernetes"
)

// TODO: replace this with generic backend interface for other options (PV, Azure)
// handleS3 saves etcd cluster's backup to specificed S3 path.
func handleS3(kubecli kubernetes.Interface, s *api.S3BackupSource, endpoints []string, clientTLSSecret, namespace string) error {
	cli, err := s3factory.NewClientFromSecret(kubecli, namespace, s.AWSSecret)
	if err != nil {
		return err
	}
	defer cli.Close()

	var tlsConfig *tls.Config
	if len(clientTLSSecret) != 0 {
		d, err := k8sutil.GetTLSDataFromSecret(kubecli, namespace, clientTLSSecret)
		if err != nil {
			return fmt.Errorf("failed to get TLS data from secret (%v): %v", clientTLSSecret, err)
		}
		tlsConfig, err = etcdutil.NewTLSConfig(d.CertData, d.KeyData, d.CAData)
		if err != nil {
			return fmt.Errorf("failed to constructs tls config: %v", err)
		}
	}

	bm := backup.NewBackupManagerFromWriter(kubecli, writer.NewS3Writer(cli.S3), tlsConfig, endpoints, namespace)
	err = bm.SaveSnap(s.Path)
	if err != nil {
		return fmt.Errorf("failed to save snapshot (%v)", err)
	}
	return nil
}
