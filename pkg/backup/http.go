// Copyright 2016 The kube-etcd-controller Authors
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
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/pkg/util/constants"
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
	files, err := ioutil.ReadDir(constants.BackupDir)
	if err != nil {
		logrus.Errorf("failed to list dir (%s): error (%v)", constants.BackupDir, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fname := getLatestSnapshotName(files)
	if len(fname) == 0 {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	http.ServeFile(w, r, path.Join(constants.BackupDir, fname))
}

func getLatestSnapshotName(files []os.FileInfo) string {
	maxRev := int64(0)
	fname := ""
	for _, file := range files {
		base := filepath.Base(file.Name())
		s := strings.Split(base, ".")[0]
		rev, err := strconv.ParseInt(s, 16, 64)
		if err != nil {
			logrus.Errorf("failed to understand snapshot name (%s): error (%v)", file.Name(), err)
			continue
		}
		if rev > maxRev {
			maxRev = rev
			fname = base
		}
	}
	return fname
}
