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
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/etcd-operator/pkg/util"

	"github.com/Sirupsen/logrus"
)

func (b *Backup) startHTTP() {
	http.HandleFunc("/backup", b.serveSnap)
	http.HandleFunc("/backupnow", b.serveBackupNow)

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
	fname, rc, err := b.be.getLatest()
	if err != nil {
		logrus.Errorf("fail to serve backup: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(fname) == 0 {
		http.NotFound(w, r)
		return
	}

	serV, err := getVersionFromBackupName(fname)
	if err != nil {
		http.Error(w, fmt.Sprintf("fail to parse etcd version from file (%s): %v", fname, err), http.StatusInternalServerError)
		return
	}

	versionValue := r.FormValue(util.BackupHTTPQueryVersion)
	reqV, err := getMajorAndMinorVersion(strings.TrimLeft(versionValue, "v"))
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid param 'version' (%s): %v", versionValue, err), http.StatusBadRequest)
		return
	}

	if !isVersionCompatible(reqV, serV) {
		http.Error(w, fmt.Sprintf("requested version (%s) is not compatible with the backup (%s)", reqV, serV), http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodHead {
		return
	}

	_, err = io.Copy(w, rc)
	if err != nil {
		logrus.Errorf("failed to write backup to %s: %v", r.RemoteAddr, err)
	}
}
