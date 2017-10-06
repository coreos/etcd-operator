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

package backend

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/backup/util"

	"github.com/sirupsen/logrus"
)

// ensure s3Backend satisfies backend interface.
var _ Backend = &s3Backend{}

// s3Backend is AWS S3 backend.
type s3Backend struct {
	s3 *s3.S3
	// dir to temporarily store backup files before upload it to S3.
	dir string
}

func NewS3Backend(s3 *s3.S3, dir string) Backend {
	return &s3Backend{s3, dir}
}

func (sb *s3Backend) Save(version string, snapRev int64, rc io.Reader) (int64, error) {
	// make a local file copy of the backup first, since s3 requires io.ReadSeeker.
	key := util.MakeBackupName(version, snapRev)
	tmpfile, err := os.OpenFile(filepath.Join(sb.dir, key), os.O_RDWR|os.O_CREATE, util.BackupFilePerm)
	if err != nil {
		return -1, fmt.Errorf("failed to create snapshot tempfile: %v", err)
	}
	defer func() {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
	}()

	n, err := io.Copy(tmpfile, rc)
	if err != nil {
		return -1, fmt.Errorf("failed to save snapshot to tmpfile: %v", err)
	}
	_, err = tmpfile.Seek(0, os.SEEK_SET)
	if err != nil {
		return -1, err
	}
	// S3 put is atomic, so let's go ahead and put the key directly.
	err = sb.s3.Put(key, tmpfile)
	if err != nil {
		return -1, err
	}
	logrus.Infof("saved backup %s (size: %d) successfully", key, n)
	return n, nil
}

func (sb *s3Backend) GetLatest() (string, error) {
	keys, err := sb.s3.List()
	if err != nil {
		return "", fmt.Errorf("failed to list s3 bucket: %v", err)
	}

	return util.GetLatestBackupName(keys), nil
}

func (sb *s3Backend) Open(name string) (io.ReadCloser, error) {
	return sb.s3.Get(name)
}

func (sb *s3Backend) Purge(maxBackupFiles int) error {
	names, err := sb.s3.List()
	if err != nil {
		return err
	}
	bnames := util.FilterAndSortBackups(names)
	if len(bnames) < maxBackupFiles {
		return nil
	}
	for i := 0; i < len(bnames)-maxBackupFiles; i++ {
		err := sb.s3.Delete(bnames[i])
		if err != nil {
			logrus.Errorf("fail to delete s3 file (%s): %v", bnames[i], err)
		}
	}
	return nil
}

func (sb *s3Backend) Total() (int, error) {
	names, err := sb.s3.List()
	if err != nil {
		return -1, err
	}
	return len(util.FilterAndSortBackups(names)), nil
}

func (sb *s3Backend) TotalSize() (int64, error) {
	return sb.s3.TotalSize()
}
