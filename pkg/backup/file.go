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
	"os"
	"path"
	"path/filepath"

	"github.com/Sirupsen/logrus"
)

const (
	backupTmpDir         = "tmp"
	backupFilePerm       = 0600
	backupFilenameSuffix = "etcd.backup"
)

// ensure fileBackend satisfies backend interface.
var _ backend = &fileBackend{}

type fileBackend struct {
	dir string
}

func (fb *fileBackend) save(version string, snapRev int64, rc io.Reader) error {
	filename := makeBackupName(version, snapRev)
	tmpfile, err := os.OpenFile(filepath.Join(fb.dir, backupTmpDir, filename), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, backupFilePerm)
	if err != nil {
		return fmt.Errorf("failed to create snapshot tempfile: %v", err)
	}
	n, err := io.Copy(tmpfile, rc)
	if err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		return fmt.Errorf("failed to save snapshot: %v", err)
	}
	err = tmpfile.Sync()
	if err != nil {
		logrus.Warningf("filebackend: failed to sync backup file %s (%v)", filename, err)
	}
	tmpfile.Close()

	nextSnapshotName := filepath.Join(fb.dir, filename)
	err = os.Rename(tmpfile.Name(), nextSnapshotName)
	if err != nil {
		os.Remove(tmpfile.Name())
		return fmt.Errorf("rename snapshot from %s to %s failed: %v", tmpfile.Name(), nextSnapshotName, err)
	}

	logrus.Infof("saved snapshot %s (size: %d) successfully", nextSnapshotName, n)
	return nil
}

func (fb *fileBackend) getLatest() (string, io.ReadCloser, error) {
	files, err := ioutil.ReadDir(fb.dir)
	if err != nil {
		return "", nil, fmt.Errorf("failed to list dir (%s): error (%v)", fb.dir, err)
	}

	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}

	fn := getLatestBackupName(names)
	if fn == "" {
		return "", nil, nil
	}
	f, err := os.Open(path.Join(fb.dir, fn))
	return fn, f, err
}

func (fb *fileBackend) purge(maxBackupFiles int) error {
	files, err := ioutil.ReadDir(fb.dir)
	if err != nil {
		return err
	}

	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}

	bnames := filterAndSortBackups(names)
	if len(bnames) < maxBackupFiles {
		return nil
	}
	for i := 0; i < len(bnames)-maxBackupFiles; i++ {
		err := os.Remove(path.Join(fb.dir, bnames[i]))
		if err != nil {
			logrus.Errorf("failed to remove backup file (%s): %v", bnames[i], err)
		} else {
			logrus.Infof("removed backup file: %s", bnames[i])
		}
	}
	return nil
}
