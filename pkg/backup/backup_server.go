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

package backup

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/coreos/etcd-operator/pkg/backup/backend"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/backup/util"

	"github.com/sirupsen/logrus"
)

// BackupServer provides the http handlers to handle backup related requests.
type BackupServer struct {
	backend backend.Backend
}

// ServeBackup serves the backup request:
// - For GET, it returns the headers of etcd version and revision, with backup data.
//   For HEAD, it only returns the headers.
// - It checks compatibility and fails any incompatible backup request
// - If both etcd version and revision, it returns the specified backup.
//   If etcd version and revision is not given, it returns the latest compatible backup.
//   If etcd version is not given, it returns the latest backup.
func (bs *BackupServer) ServeBackup(w http.ResponseWriter, r *http.Request) {
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
		fname, err = bs.backend.GetLatest()
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

	rc, err := bs.backend.Open(fname)
	if err != nil {
		// TODO: define backend layer not found error
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
