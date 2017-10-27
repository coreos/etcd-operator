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

package backupstorage

import (
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	backups3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"

	"k8s.io/client-go/kubernetes"
)

type s3 struct {
	clusterName  string
	namespace    string
	s3Prefix     string
	s3Client     *s3factory.S3Client
	backupPolicy api.BackupPolicy
	kubecli      kubernetes.Interface
	s3cli        *backups3.S3
}

func NewS3Storage(kubecli kubernetes.Interface, clusterName, ns string, p api.BackupPolicy) (Storage, error) {
	cli, err := s3factory.NewClientFromSecret(kubecli, ns, p.S3.AWSSecret)
	if err != nil {
		return nil, err
	}

	prefix := backupapi.ToS3Prefix(p.S3.Prefix, ns, clusterName)
	s3cli := backups3.NewFromClient(p.S3.S3Bucket, prefix, cli.S3)

	s := &s3{
		kubecli:      kubecli,
		clusterName:  clusterName,
		backupPolicy: p,
		namespace:    ns,
		s3Prefix:     p.S3.Prefix,
		s3cli:        s3cli,
		s3Client:     cli,
	}
	return s, nil
}

func (s *s3) Create() error {
	// TODO: check if bucket/folder exists?
	return nil
}

func (s *s3) Clone(fromCluster string) error {
	return s.s3cli.CopyPrefix(backupapi.ToS3Prefix(s.s3Prefix, s.namespace, fromCluster))
}

func (s *s3) Delete() error {
	if s.backupPolicy.AutoDelete {
		names, err := s.s3cli.List()
		if err != nil {
			return err
		}
		for _, n := range names {
			err = s.s3cli.Delete(n)
			if err != nil {
				return err
			}
		}
	}
	if s.backupPolicy.S3 != nil {
		s.s3Client.Close()
	}
	return nil
}
