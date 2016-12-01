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
	"testing"

	"path/filepath"

	"github.com/coreos/etcd-operator/pkg/util"
)

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
		{"v3.0.15", http.StatusOK},
		{"v3.0.0", http.StatusOK},
		{"v3.1.0-alpha.1", http.StatusOK},
		{"v3.1.0", http.StatusOK},
		{"v3.2.0", http.StatusBadRequest},
		{"v2.3.7", http.StatusBadRequest},
		{"", http.StatusBadRequest},
	}

	for i, tt := range tests {
		b := &Backup{
			be: &fileBackend{dir: d},
		}
		req := &http.Request{
			URL: util.MakeBackupURL("ignore", tt.reqVersion),
		}
		rr := httptest.NewRecorder()
		b.serveSnap(rr, req)

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
