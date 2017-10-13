// Copyright 2016 The etcd-operator Authors
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

package backup

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/backup/util"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	HTTPHeaderEtcdVersion = "X-etcd-Version"
	HTTPHeaderRevision    = "X-Revision"
)

// StartHTTP starts to listen for incoming backup http requests.
func (bc *BackupController) StartHTTP() {
	http.HandleFunc(backupapi.APIV1+"/backup", bc.backupServer.ServeBackup)
	http.HandleFunc(backupapi.APIV1+"/backupnow", bc.serveBackupNow)
	http.HandleFunc(backupapi.APIV1+"/status", bc.serveStatus)
	http.Handle("/metrics", prometheus.Handler())

	logrus.Infof("listening on %v", bc.listenAddr)
	panic(http.ListenAndServe(bc.listenAddr, nil))
}

type backupNowAck struct {
	err    error
	status backupapi.BackupStatus
}

func (bc *BackupController) serveBackupNow(w http.ResponseWriter, r *http.Request) {
	ackchan := make(chan backupNowAck, 1)
	select {
	case bc.backupNow <- ackchan:
	case <-time.After(time.Minute):
		http.Error(w, "timeout", http.StatusRequestTimeout)
		return
	}

	select {
	case ack := <-ackchan:
		if ack.err != nil {
			http.Error(w, ack.err.Error(), http.StatusInternalServerError)
			return
		}
		e := json.NewEncoder(w)
		err := e.Encode(ack.status)
		if err != nil {
			logrus.Errorf("failed to write backup status: %v", err)
		}
	case <-time.After(10 * time.Minute):
		http.Error(w, "timeout", http.StatusRequestTimeout)
		return
	}
}

func (bc *BackupController) serveStatus(w http.ResponseWriter, r *http.Request) {
	t, err := bc.backupManager.be.Total()
	if err != nil {
		http.Error(w, "failed to get total number of backups", http.StatusInternalServerError)
		return
	}
	ts, err := bc.backupManager.be.TotalSize()
	if err != nil {
		http.Error(w, "failed to get total size of backups", http.StatusInternalServerError)
		return
	}
	s := backupapi.ServiceStatus{
		Backups:    t,
		BackupSize: util.ToMB(ts),
	}
	rbs := bc.recentBackupsStatus
	if len(rbs) != 0 {
		s.RecentBackup = &rbs[len(rbs)-1]
	}

	je := json.NewEncoder(w)
	if err := je.Encode(&s); err != nil {
		logrus.Errorf("failed to write service status to %s: %v", r.RemoteAddr, err)
	}
}
