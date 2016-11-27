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
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreos/etcd-operator/pkg/util"
	"github.com/coreos/go-semver/semver"

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
	files, err := ioutil.ReadDir(b.backupDir)
	if err != nil {
		logrus.Errorf("failed to list dir (%s): error (%v)", b.backupDir, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fname := getLatestSnapshotName(files)
	if len(fname) == 0 {
		http.NotFound(w, r)
		return
	}

	versionValue := r.FormValue(util.BackupHTTPQueryVersion)
	reqV, err := getMajorAndMinorVersion(strings.TrimLeft(versionValue, "v"))
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid param 'version' (%s): %v", versionValue, err), http.StatusBadRequest)
		return
	}
	serV, err := getEtcdVersionFromSnap(fname)
	if err != nil {
		http.Error(w, fmt.Sprintf("fail to parse etcd version from file (%s): %v", fname, err), http.StatusInternalServerError)
		return
	}
	if !isVersionCompatible(reqV, serV) {
		http.Error(w, fmt.Sprintf("requested version (%s) is not compatible with the backup (%s)", reqV, serV), http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodHead {
		return
	}
	http.ServeFile(w, r, path.Join(b.backupDir, fname))
}

func isVersionCompatible(req, serve string) bool {
	compatVersions, ok := compatibilityMap[req]
	if !ok {
		return false
	}
	_, ok = compatVersions[serve]
	return ok
}

// getMajorAndMinorVersion expects a semver and then returns "major.minor"
func getMajorAndMinorVersion(rawV string) (string, error) {
	v, err := semver.NewVersion(rawV)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor), nil
}

func getLatestSnapshotName(files []os.FileInfo) string {
	maxRev := int64(0)
	fname := ""
	for _, file := range files {
		base := filepath.Base(file.Name())
		if !isBackup(base) {
			continue
		}
		rev, err := getRev(base)
		if err != nil {
			logrus.Errorf("fail to get rev from backup (%s): %v", file.Name(), err)
			continue
		}
		if rev > maxRev {
			maxRev = rev
			fname = base
		}
	}
	return fname
}

func isBackup(filename string) bool {
	return strings.HasSuffix(filename, backupFilenameSuffix)
}
