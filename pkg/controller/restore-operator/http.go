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

	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/sirupsen/logrus"
)

const (
	backupHTTPPath = "/backup"
	listenAddr = "0.0.0.0:19999"
)

func (r *Restore) startHTTP() {
	http.HandleFunc(backupapi.APIV1+"/backup", r.serveBackup)
	logrus.Infof("listening on %v", listenAddr)
	panic(http.ListenAndServe(listenAddr, nil))
}

func (r *Restore) serveBackup(w http.ResponseWriter, req *http.Request) {
	clusterName := req.URL.Query().Get("cluster")
	if len(clusterName) == 0 {
		http.Error(w, "cluster is not specified", http.StatusBadRequest)
		return
	}
	v, ok := r.backupServers.Load(clusterName)
	if !ok {
		http.Error(w, fmt.Sprintf("cluster %v backup server not found", clusterName), http.StatusInternalServerError)
		return
	}
	go func() {
		logrus.Infof("serving backup for cluster %v", clusterName)
		bs := v.(*backupServer)
		bs.ServeBackup(w, req)
		bs.close()
		r.backupServers.Delete(clusterName)
		logrus.Infof("serving backup for cluster %v done", clusterName)
	}()
)

// BackupServer is restore operator specific http server that
// wraps around backup.BackupServer to provide:
// - additional <clusterName> path parsing and business logic.
// - Close() method for cleanup.
type BackupServer struct {
	*backup.BackupServer
	backupServers *sync.Map
}

// path: /backup/<cluster-name>
func (bs *BackupServer) serveBackup(w http.ResponseWriter, r *http.Request) {
	clusterName := r.URL.Path[len(backupHTTPPath+"/")]
	v, ok := bs.backupServers.Load(clusterName)
	if !ok {
		// return Not Found
	}
	b := v.(*backup.BackupServer)
	b.ServeBackup(w, r)
}
