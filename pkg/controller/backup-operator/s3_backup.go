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
	"fmt"
	"path"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/backup/writer"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"

	"k8s.io/client-go/kubernetes"
)

const backupName = "etcd.backup"

// TODO: replace this with generic backend interface for other options (PV, Azure)
// handleS3 backups up etcd cluster to s3 and return s3 path for the backup file.
func handleS3(kubecli kubernetes.Interface, s3 *api.S3Source, namespace, clusterName string) (string, error) {
	cli, err := s3factory.NewClientFromSecret(kubecli, namespace, s3.AWSSecret)
	if err != nil {
		return "", err
	}
	defer cli.Close()
	prefix := backupapi.ToS3Prefix(s3.Prefix, namespace, clusterName)
	// TODO: support TLS.
	bm := backup.NewBackupManagerFromWriter(kubecli, nil, writer.NewS3Writer(cli.S3), clusterName, namespace)
	fullS3Path := toS3Path(s3.S3Bucket, prefix, backupName)
	err = bm.SaveSnapWithPath(fullS3Path)
	if err != nil {
		return "", fmt.Errorf("failed to save snapshot (%v)", err)
	}
	return fullS3Path, nil
}

// toS3Path returns the full s3 path of the backup file based s3Bucket, prefix, and backupName.
func toS3Path(s3Bucket, prefix, backupName string) string {
	return path.Join(s3Bucket, prefix, backupName)
}
