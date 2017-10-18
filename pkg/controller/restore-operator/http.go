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
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/controller/controllerutil"

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

// serveBackup parses incoming request url of the form /backup/<cluster-name>
// get the etcd cluster name.
// Then it returns the etcd cluster backup snapshot to the caller.
func (r *Restore) serveBackup(w http.ResponseWriter, req *http.Request) {
	clusterName := string(req.URL.Path[len(backupHTTPPath):])
	if len(clusterName) == 0 {
		http.Error(w, "cluster is not specified", http.StatusBadRequest)
		return
	}
	v, ok := r.restoreCRs.Load(clusterName)
	if !ok {
		http.Error(w, fmt.Sprintf("cluster %v backup server not found", clusterName), http.StatusInternalServerError)
		return
	}

	logrus.Infof("serving backup for cluster %v", clusterName)
	cr := v.(*api.EtcdRestore)
	spec := cr.Spec.BackupSpec
	switch spec.StorageType {
	case api.BackupStorageTypeS3:
		s3Spec := spec.S3
		be, err := controllerutil.NewS3backend(r.kubecli, s3Spec, r.namespace, clusterName)
		if err != nil {
			http.Error(w, "failed to create s3 backend", http.StatusInternalServerError)
			return
		}
		bs := backup.NewBackupServer(be)
		bs.ServeBackup(w, req)
		be.Close()
	default:
		http.Error(w, fmt.Sprintf("unknown storage type %v", spec.StorageType), http.StatusInternalServerError)
	}
}
