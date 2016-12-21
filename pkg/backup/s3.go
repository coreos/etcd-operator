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
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd-operator/pkg/backup/s3"
)

// ensure s3Backend satisfies backend interface.
var _ backend = &s3Backend{}

type s3Backend struct {
	S3 *s3.S3
	// dir to temporarily store backup files before upload it to S3.
	dir string
}

func (sb *s3Backend) save(version string, snapRev int64, rc io.Reader) error {
	// make a local file copy of the backup first, since s3 requires io.ReadSeeker.
	key := makeBackupName(version, snapRev)
	tmpfile, err := os.OpenFile(filepath.Join(sb.dir, key), os.O_RDWR|os.O_CREATE, backupFilePerm)
	if err != nil {
		return fmt.Errorf("failed to create snapshot tempfile: %v", err)
	}
	defer func() {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
	}()

	n, err := io.Copy(tmpfile, rc)
	if err != nil {
		return fmt.Errorf("failed to save snapshot: %v", err)
	}
	_, err = tmpfile.Seek(0, os.SEEK_SET)
	if err != nil {
		return err
	}
	// S3 put is atomic, so let's go ahead and put the key directly.
	err = sb.S3.Put(key, tmpfile)
	if err != nil {
		return err
	}
	logrus.Infof("saved backup %s (size: %d) successfully", key, n)
	return nil
}

func (sb *s3Backend) getLatest() (string, io.ReadCloser, error) {
	keys, err := sb.S3.List()
	if err != nil {
		return "", nil, fmt.Errorf("failed to list s3 bucket: %v", err)
	}

	key := getLatestBackupName(keys)
	if key == "" {
		return "", nil, nil
	}
	rc, err := sb.S3.Get(key)

	return key, rc, err
}

func (sb *s3Backend) purge(maxBackupFiles int) error {
	names, err := sb.S3.List()
	if err != nil {
		return err
	}
	bnames := filterAndSortBackups(names)
	if len(bnames) < maxBackupFiles {
		return nil
	}
	for i := 0; i < len(bnames)-maxBackupFiles; i++ {
		err := sb.S3.Delete(bnames[i])
		if err != nil {
			logrus.Errorf("fail to delete s3 file (%s): %v", bnames[i], err)
		}
	}
	return nil
}
