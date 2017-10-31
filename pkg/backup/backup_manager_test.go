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
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/coreos/etcd-operator/pkg/backup/backend"
	"github.com/coreos/etcd-operator/pkg/backup/util"
	"github.com/coreos/etcd/clientv3"
)

const (
	testEtcdVersion = "3.1.8"
	testData        = "foo"
)

type fakeMaintenanceClient struct {
	clientv3.Maintenance
}

func (c *fakeMaintenanceClient) Snapshot(ctx context.Context) (io.ReadCloser, error) {
	return ioutil.NopCloser(bytes.NewReader([]byte(testData))), nil
}

func (c *fakeMaintenanceClient) Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error) {
	return &clientv3.StatusResponse{Version: testEtcdVersion}, nil
}

// TestWriteSnap ensures BackupManager.WriteSnap can write snapshot to
// the local file backend and return a correct corresponding backup status.
func TestWriteSnap(t *testing.T) {
	var rev int64 = 1
	bn := util.MakeBackupName(testEtcdVersion, rev)
	d, err := makeFileBackendDir(bn)
	if err != nil {
		t.Fatalf("failed to make file backend dir: (%v)", err)
	}
	bm := &BackupManager{
		be: backend.NewFileBackend(d),
	}

	bs, err := bm.writeSnap(&fakeMaintenanceClient{}, "", rev)
	if err != nil {
		t.Fatal(err)
	}
	if bs.Version != testEtcdVersion {
		t.Fatalf("expect Version %v, got %v", testEtcdVersion, bs.Version)
	}
	if bs.Revision != rev {
		t.Fatalf("expect Version %v, got %v", rev, bs.Version)
	}

	lbn, err := bm.be.GetLatest()
	if err != nil {
		t.Fatal(err)
	}
	if bn != lbn {
		t.Fatalf("expect backup name %v, got %v", bn, lbn)
	}
	rc, err := bm.be.Open(lbn)
	if err != nil {
		t.Fatalf("failed to open %v: (%v) ", lbn, err)
	}
	sd, err := ioutil.ReadAll(rc)
	if err != nil {
		t.Fatalf("failed to read %v: (%v) ", lbn, err)
	}
	sds := string(sd)
	if sds != testData {
		t.Fatalf("expect saved data %v, got (%v) ", testData, sds)
	}
}

func makeFileBackendDir(snap string) (string, error) {
	d, err := ioutil.TempDir("", "backupdir")
	if err != nil {
		return "", err
	}
	// file backend searches under "backupdir/tmp" for snap file.
	// hence, creating "backupdir/tmp" dir here ensures file backend
	// can find the correct snap path.
	tmpDir := filepath.Join(d, util.BackupTmpDir)
	err = os.MkdirAll(tmpDir, os.ModePerm)
	if err != nil {
		return "", err
	}
	f := filepath.Join(tmpDir, snap)
	if err := ioutil.WriteFile(f, []byte("ignored"), 0644); err != nil {
		return "", err
	}
	return d, nil
}
