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

package backend

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/coreos/etcd-operator/pkg/backup/abs"
	"github.com/coreos/etcd-operator/pkg/backup/util"

	"github.com/sirupsen/logrus"
)

// ensure absBackend satisfies backend interface.
var _ Backend = &absBackend{}

// absBackend is the Azure Blob Storage backend.
type absBackend struct {
	ABS *abs.ABS
}

func NewAbsBackend(abs *abs.ABS) Backend {
	return &absBackend{abs}
}

func (ab *absBackend) Save(version string, snapRev int64, r io.Reader) (int64, error) {
	key := util.MakeBackupName(version, snapRev)

	err := ab.ABS.Put(key, r)
	if err != nil {
		return -1, err
	}

	n, err := ab.getBlobSize(key)
	if err != nil {
		return -1, err
	}

	logrus.Infof("saved backup %s (size: %d) successfully", key, n)
	return n, nil
}

func (ab *absBackend) GetLatest() (string, error) {
	keys, err := ab.ABS.List()
	if err != nil {
		return "", fmt.Errorf("failed to list abs container: %v", err)
	}

	return util.GetLatestBackupName(keys), nil
}

func (ab *absBackend) Open(name string) (io.ReadCloser, error) {
	return ab.ABS.Get(name)
}

func (ab *absBackend) Purge(maxBackupFiles int) error {
	names, err := ab.ABS.List()
	if err != nil {
		return err
	}
	bnames := util.FilterAndSortBackups(names)
	if len(bnames) < maxBackupFiles {
		return nil
	}
	for i := 0; i < len(bnames)-maxBackupFiles; i++ {
		err := ab.ABS.Delete(bnames[i])
		if err != nil {
			logrus.Errorf("fail to delete abs blob (%s): %v", bnames[i], err)
		}
	}
	return nil
}

func (ab *absBackend) Total() (int, error) {
	names, err := ab.ABS.List()
	if err != nil {
		return -1, err
	}
	return len(util.FilterAndSortBackups(names)), err
}

func (ab *absBackend) TotalSize() (int64, error) {
	return ab.ABS.TotalSize()
}

func (ab *absBackend) getBlobSize(key string) (int64, error) {
	rc, err := ab.Open(key)

	// TODO: use less expensive method of getting byte array length
	b, err := ioutil.ReadAll(rc)
	if err != nil {
		rc.Close()
		return -1, err
	}

	return int64(len(b)), rc.Close()
}
