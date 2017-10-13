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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/coreos/etcd-operator/pkg/backup/backend"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
)

func TestRespHeaderHasVersionRevision(t *testing.T) {
	d, err := setupBackupDir("3.1.0_000000000000000a_etcd.backup")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)
	bs := &BackupServer{
		backend: backend.NewFileBackend(d),
	}
	req := &http.Request{
		URL: backupapi.NewBackupURL("http", "ignore", "", -1),
	}
	rr := httptest.NewRecorder()
	bs.ServeBackup(rr, req)
	if get := rr.Header().Get(HTTPHeaderEtcdVersion); get != "3.1.0" {
		t.Errorf("etcd version want=%s, get=%s", "3.1.0", get)
	}
	if get := rr.Header().Get(HTTPHeaderRevision); get != "10" {
		t.Errorf("revision want=%s, get=%s", "10", get)
	}
}

func TestServeBackup(t *testing.T) {
	d, err := setupBackupDir("3.0.15_0000000000000002_etcd.backup")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)

	tests := []struct {
		reqVersion  string
		reqRevision int64
		httpC       int
	}{
		{"3.0.15", 2, http.StatusOK},
		{"3.0.0", 2, http.StatusNotFound},
		{"3.0.15", 3, http.StatusNotFound},
		{"", 3, http.StatusBadRequest},
	}

	for i, tt := range tests {
		bs := &BackupServer{
			backend: backend.NewFileBackend(d),
		}
		req := &http.Request{
			URL: backupapi.NewBackupURL("http", "ignore", tt.reqVersion, tt.reqRevision),
		}
		rr := httptest.NewRecorder()
		bs.ServeBackup(rr, req)

		if rr.Code != tt.httpC {
			if rr.Code != http.StatusOK {
				b, _ := ioutil.ReadAll(rr.Body)
				t.Log(string(b))
			}
			t.Errorf("#%d: http code want = %d, get = %d", i, tt.httpC, rr.Code)
		}
	}
}

func TestBackupVersionCompatiblity(t *testing.T) {
	d, err := setupBackupDir("3.0.15_0000000000000002_etcd.backup")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)

	tests := []struct {
		reqVersion string
		httpC      int
	}{
		{"3.0.15", http.StatusOK},
		{"3.0.0", http.StatusOK},
		{"3.1.0-alpha.1", http.StatusOK},
		{"3.1.0", http.StatusOK},
		{"3.2.0", http.StatusBadRequest},
		{"2.3.7", http.StatusBadRequest},
		{"", http.StatusOK},
	}

	for i, tt := range tests {
		bs := &BackupServer{
			backend: backend.NewFileBackend(d),
		}
		req := &http.Request{
			URL: backupapi.NewBackupURL("http", "ignore", tt.reqVersion, -1),
		}
		rr := httptest.NewRecorder()
		bs.ServeBackup(rr, req)

		if rr.Code != tt.httpC {
			if rr.Code != http.StatusOK {
				b, _ := ioutil.ReadAll(rr.Body)
				t.Log(string(b))
			}
			t.Errorf("#%d: http code want = %d, get = %d", i, tt.httpC, rr.Code)
		}
	}
}

func setupBackupDir(snap string) (string, error) {
	d, err := ioutil.TempDir("", "backupdir")
	if err != nil {
		return "", err
	}
	f := filepath.Join(d, snap)
	if err := ioutil.WriteFile(f, []byte("ignored"), 0644); err != nil {
		return "", err
	}
	return d, nil
}
