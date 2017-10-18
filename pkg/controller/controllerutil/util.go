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

package controllerutil

import (
	"fmt"
	"io/ioutil"
	"os"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup/backend"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	backupS3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/cluster/backupstorage"

	"k8s.io/client-go/kubernetes"
)

const (
	tmpdir = "/tmp"
)

// S3backend contains a s3Backend and all its created resources.
// call Close() to releases those created resources.
type S3backend struct {
	backend.Backend
	awsDir    string
	backupDir string
}

// Close releases resources associated with S3backend.
func (s *S3backend) Close() {
	os.RemoveAll(s.awsDir)
	os.RemoveAll(s.backupDir)
}

// NewS3backend creates a S3backend.
func NewS3backend(kubecli kubernetes.Interface, s3 *api.S3Source, namespace, clusterName string) (*S3backend, error) {
	awsDir, err := ioutil.TempDir(tmpdir, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create aws config/cred dir: (%v)", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(awsDir)
		}
	}()
	so, err := backupstorage.SetupAWSConfig(kubecli, namespace, s3.AWSSecret, awsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to setup aws config: (%v)", err)
	}

	prefix := backupapi.ToS3Prefix(s3.Prefix, namespace, clusterName)
	s3cli, err := backupS3.NewFromSessionOpt(s3.S3Bucket, prefix, *so)
	if err != nil {
		return nil, fmt.Errorf("failed to create aws cli: (%v)", err)
	}

	backupDir, err := ioutil.TempDir(tmpdir, "")
	if err != nil {
		return nil, err
	}
	return &S3backend{
		Backend:   backend.NewS3Backend(s3cli, backupDir),
		awsDir:    awsDir,
		backupDir: backupDir,
	}, nil
}
