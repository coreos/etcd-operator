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
	"strconv"
	"time"

	"github.com/coreos/etcd-operator/pkg/backup/backupapi"

	"github.com/Sirupsen/logrus"
)

const (
	HTTPHeaderEtcdVersion = "X-etcd-Version"
	HTTPHeaderRevision    = "X-Revision"
)

func (b *Backup) startHTTP() {
	http.HandleFunc(backupapi.APIV1+"/backup", b.serveSnap)
	http.HandleFunc(backupapi.APIV1+"/backupnow", b.serveBackupNow)
	http.HandleFunc(backupapi.APIV1+"/status", b.serveStatus)

	logrus.Infof("listening on %v", b.listenAddr)
	panic(http.ListenAndServe(b.listenAddr, nil))
}

func (b *Backup) serveBackupNow(w http.ResponseWriter, r *http.Request) {
	ackchan := make(chan error, 1)
	select {
	case b.backupNow <- ackchan:
	case <-time.After(time.Minute):
		http.Error(w, "timeout", http.StatusRequestTimeout)
		return
	}

	select {
	case err := <-ackchan:
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	case <-time.After(10 * time.Minute):
		http.Error(w, "timeout", http.StatusRequestTimeout)
		return
	}
}

func (b *Backup) serveSnap(w http.ResponseWriter, r *http.Request) {
	fname, err := b.be.getLatest()
	if err != nil {
		logrus.Errorf("fail to serve backup: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(fname) == 0 {
		http.NotFound(w, r)
		return
	}
	rc, err := b.be.open(fname)
	if err != nil {
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

	checkVersion := r.FormValue(backupapi.HTTPQueryVersionKey)
	// If version param is empty, we don't need to check compatibility.
	// This could happen if user manually requests it.
	if len(checkVersion) != 0 {
		reqV, err := getMajorAndMinorVersion(checkVersion)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid param 'version' (%s): %v", checkVersion, err), http.StatusBadRequest)
			return
		}

		if !isVersionCompatible(reqV, serV) {
			http.Error(w, fmt.Sprintf("requested version (%s) is not compatible with the backup (%s)", reqV, serV), http.StatusBadRequest)
			return
		}
	}

	w.Header().Set(HTTPHeaderEtcdVersion, getVersionFromBackup(fname))
	rev, err := getRev(fname)
	if err != nil {
		panic("unexpected error:" + err.Error()) // fname should have already been verified
	}
	w.Header().Set(HTTPHeaderRevision, strconv.FormatInt(rev, 10))

	if r.Method == http.MethodHead {
		return
	}

	_, err = io.Copy(w, rc)
	if err != nil {
		logrus.Errorf("failed to write backup to %s: %v", r.RemoteAddr, err)
	}
}

func (b *Backup) serveStatus(w http.ResponseWriter, r *http.Request) {
	t, err := b.be.total()
	if err != nil {
		http.Error(w, "failed to get total number of backups", http.StatusInternalServerError)
		return
	}
	ts, err := b.be.totalSize()
	if err != nil {
		http.Error(w, "failed to get total size of backups", http.StatusInternalServerError)
		return
	}
	s := backupapi.ServiceStatus{
		Backups:    t,
		BackupSize: toMB(ts),
	}
	if len(b.recentBackupsStatus) != 0 {
		s.RecentBackup = &b.recentBackupsStatus[len(b.recentBackupsStatus)-1]
	}

	je := json.NewEncoder(w)
	if err := je.Encode(&s); err != nil {
		logrus.Errorf("failed to write service status to %s: %v", r.RemoteAddr, err)
	}
}
