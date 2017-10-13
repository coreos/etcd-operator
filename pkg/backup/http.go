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
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
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

func (bc *BackupController) startHTTP() {
	http.HandleFunc(backupapi.APIV1+"/backup", bc.serveSnap)
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

func (bc *BackupController) serveSnap(w http.ResponseWriter, r *http.Request) {
	var (
		fname string
		err   error
	)

	revision := r.FormValue(backupapi.HTTPQueryRevisionKey)
	version := r.FormValue(backupapi.HTTPQueryVersionKey)

	switch {
	case len(revision) != 0 && len(version) != 0:
		revisioni, err := strconv.ParseInt(revision, 10, 64)
		if err != nil {
			http.Error(w, "revision is not a vaild integer", http.StatusBadRequest)
			return
		}

		fname = util.MakeBackupName(version, revisioni)

	case len(revision) == 0:
		fname, err = bc.backupManager.be.GetLatest()
		if err != nil {
			logrus.Errorf("fail to serve backup: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(fname) == 0 {
			http.NotFound(w, r)
			return
		}

	default:
		http.Error(w, "version must be provided when revision is provided.", http.StatusBadRequest)
		return
	}

	rc, err := bc.backupManager.be.Open(fname)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "backup not found", http.StatusNotFound)
			return
		}

		logrus.Errorf("fail to open backup (%s): %v", fname, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	serV, err := getMajorMinorVersionFromBackup(fname)
	if err != nil {
		http.Error(w, fmt.Sprintf("fail to parse etcd version from file (%s): %v", fname, err), http.StatusInternalServerError)
		return
	}

	// If version param is empty, we don't need to check compatibility.
	// This could happen if user manually requests it.
	if len(version) != 0 {
		reqV, err := getMajorAndMinorVersion(version)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid param 'version' (%s): %v", version, err), http.StatusBadRequest)
			return
		}

		if !isVersionCompatible(reqV, serV) {
			http.Error(w, fmt.Sprintf("requested version (%s) is not compatible with the backup (%s)", reqV, serV), http.StatusBadRequest)
			return
		}
	}

	w.Header().Set(HTTPHeaderEtcdVersion, getVersionFromBackup(fname))
	rev := util.MustParseRevision(fname)
	w.Header().Set(HTTPHeaderRevision, strconv.FormatInt(rev, 10))

	if r.Method == http.MethodHead {
		return
	}

	_, err = io.Copy(w, rc)
	if err != nil {
		logrus.Errorf("failed to write backup to %s: %v", r.RemoteAddr, err)
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
