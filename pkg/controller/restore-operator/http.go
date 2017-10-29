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
	"net/http"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup"
	"github.com/coreos/etcd-operator/pkg/backup/backend"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	backups3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sirupsen/logrus"
)

const (
	backupHTTPPath = backupapi.APIV1 + "/backup/"
	listenAddr     = "0.0.0.0:19999"
)

func (r *Restore) startHTTP() {
	http.HandleFunc(backupapi.APIV1+"/backup/", r.serveBackup)
	logrus.Infof("listening on %v", listenAddr)
	panic(http.ListenAndServe(listenAddr, nil))
}

// serveBackup parses incoming request url of the form /backup/<restore-name>
// get the etcd restore name.
// Then it returns the etcd cluster backup snapshot to the caller.
func (r *Restore) serveBackup(w http.ResponseWriter, req *http.Request) {
	restoreName := string(req.URL.Path[len(backupHTTPPath):])
	if len(restoreName) == 0 {
		http.Error(w, "restore name is not specified", http.StatusNotFound)
		return
	}

	obj := &api.EtcdRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreName,
			Namespace: r.namespace,
		},
	}
	v, exists, err := r.indexer.Get(obj)
	if err != nil {
		msg := fmt.Sprintf("failed to get restore CR for restore-name (%v): %v", restoreName, err)
		logrus.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	if !exists {
		msg := fmt.Sprintf("no restore CR found for restore-name (%v)", restoreName)
		logrus.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	logrus.Infof("serving backup for restore CR %v", restoreName)
	cr := v.(*api.EtcdRestore)
	spec := cr.Spec.BackupSpec
	switch spec.StorageType {
	case api.BackupStorageTypeS3:
		bs, cli, err := r.makeBackupServer(spec.S3, spec.ClusterName)
		if err != nil {
			http.Error(w, "failed to create S3 backup server", http.StatusInternalServerError)
			return
		}

		bs.ServeBackup(w, req)
		cli.Close()
	default:
		http.Error(w, fmt.Sprintf("unknown storage type %v", spec.StorageType), http.StatusBadRequest)
	}
}

func (r *Restore) makeBackupServer(s3 *api.S3Source, clusterName string) (*backup.BackupServer, *s3factory.S3Client, error) {
	cli, err := s3factory.NewClientFromSecret(r.kubecli, r.namespace, s3.AWSSecret)
	if err != nil {
		return nil, nil, err
	}

	prefix := backupapi.ToS3Prefix(s3.Prefix, r.namespace, clusterName)
	s3cli := backups3.NewFromClient(s3.S3Bucket, prefix, cli.S3)
	be := backend.NewS3Backend(s3cli)
	bs := backup.NewBackupServer(be)
	return bs, cli, nil
}
