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
	"io"
	"net/http"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/backup/reader"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"

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
		http.Error(w, "restore name is not specified", http.StatusBadRequest)
		return
	}
	v, ok := r.restoreCRs.Load(restoreName)
	if !ok {
		http.Error(w, fmt.Sprintf("restore %v backup server not found", restoreName), http.StatusInternalServerError)
		return
	}
	cr := v.(*api.EtcdRestore)
	s3RestoreSource := cr.Spec.RestoreSource.S3
	logrus.Infof("serving backup for restore %v", restoreName)

	s3Cli, err := s3factory.NewClientFromSecret(r.kubecli, r.namespace, s3RestoreSource.AWSSecret)
	if err != nil {
		msg := fmt.Sprintf("failed to create S3 client: %v", err)
		logrus.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	defer s3Cli.Close()

	br := reader.NewS3Reader(s3Cli.S3)
	rc, err := br.Open(s3RestoreSource.Path)
	if err != nil {
		msg := fmt.Sprintf("failed to read backup file(%v): %v", s3RestoreSource.Path, err)
		logrus.Error(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	if req.Method == http.MethodHead {
		return
	}

	_, err = io.Copy(w, rc)
	if err != nil {
		logrus.Errorf("failed to write backup to %s: %v", req.RemoteAddr, err)
	}
}
