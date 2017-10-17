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
	"io/ioutil"
	"os"

	"k8s.io/client-go/kubernetes"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup"
	"github.com/coreos/etcd-operator/pkg/backup/backend"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	backupS3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/cluster/backupstorage"
	"github.com/sirupsen/logrus"
)

const tmpdir = "/tmp"

// backupServer wraps around backup.BackupServer with a close method
// that cleans up resources associated with backup.BackupServer.
type backupServer struct {
	*backup.BackupServer
	awsDir    string
	backupDir string
}

// close removes aws credential/config dir and backupDir dir which were
// created for BackupServer.
func (bsc *backupServer) close() {
	os.RemoveAll(bsc.awsDir)
	os.RemoveAll(bsc.backupDir)
}

func (r *Restore) handleBackup(spec *api.BackupSpec) error {
	var (
		bs  *backupServer
		err error
	)
	switch spec.StorageType {
	case api.BackupStorageTypeS3:
		bs, err = createS3BackupServer(r.kubecli, spec.S3, r.namespace, spec.ClusterName)
		if err != nil {
			return err
		}
	default:
		logrus.Fatalf("unknown StorageType: %v", spec.StorageType)
	}
	r.backupServers.Store(spec.ClusterName, bs)
	return nil
}

func createS3BackupServer(kubecli kubernetes.Interface, s3 *api.S3Source, namespace, clusterName string) (*backupServer, error) {
	awsDir, err := ioutil.TempDir(tmpdir, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create aws config/cred dir: (%v)", err)
	}

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

	return &backupServer{
		BackupServer: backup.NewBackupServer(backend.NewS3Backend(s3cli, backupDir)),
		awsDir:       awsDir,
		backupDir:    backupDir,
	}, nil
}
