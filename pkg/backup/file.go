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
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
)

const (
	backupTmpDir         = "tmp"
	backupFilePerm       = 0600
	backupFilenameSuffix = "etcd.backup"
)

func writeBackupFile(backupDir, version string, snapRev int64, rc io.ReadCloser) error {
	filename := makeFilename(version, snapRev)
	tmpfile, err := os.OpenFile(filepath.Join(backupDir, backupTmpDir, filename), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, backupFilePerm)
	if err != nil {
		return fmt.Errorf("failed to create snapshot tempfile: %v", err)
	}
	n, err := io.Copy(tmpfile, rc)
	if err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		return fmt.Errorf("failed to save snapshot: %v", err)
	}
	tmpfile.Close()

	nextSnapshotName := filepath.Join(backupDir, filename)
	err = os.Rename(tmpfile.Name(), nextSnapshotName)
	if err != nil {
		os.Remove(tmpfile.Name())
		return fmt.Errorf("rename snapshot from %s to %s failed: %v", tmpfile.Name(), nextSnapshotName, err)
	}
	log.Printf("saved snapshot %s (size: %d) successfully", nextSnapshotName, n)
	return nil
}

func getBackupFile(backupDir string) (string, error) {
	files, err := ioutil.ReadDir(backupDir)
	if err != nil {
		return "", fmt.Errorf("failed to list dir (%s): error (%v)", backupDir, err)
	}
	return getLatestSnapshotName(files), nil
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

func makeFilename(ver string, rev int64) string {
	return fmt.Sprintf("%s_%016x_%s", ver, rev, backupFilenameSuffix)
}

func getRev(filename string) (int64, error) {
	return strconv.ParseInt(strings.SplitN(filename, "_", 3)[1], 16, 64)
}
