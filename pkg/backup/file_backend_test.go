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
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFileBackendPurge(t *testing.T) {
	tests := []struct {
		maxFiles  int
		files     []string
		leftFiles []string
	}{{
		maxFiles: 1,
		files: []string{
			makeBackupName("3.1.0", 1),
			makeBackupName("3.1.0", 2),
		},
		leftFiles: []string{makeBackupName("3.1.0", 2)},
	}, {
		maxFiles: 1,
		files: []string{
			makeBackupName("3.1.0", 2), // ordering doesn't matter
			makeBackupName("3.1.0", 1),
		},
		leftFiles: []string{makeBackupName("3.1.0", 2)},
	}, {
		maxFiles: 1,
		files: []string{
			makeBackupName("3.1.0", 1),
			makeBackupName("3.0.0", 2), // we dont' care about version, only highest rev
		},
		leftFiles: []string{makeBackupName("3.0.0", 2)},
	}, {
		maxFiles: 1,
		files: []string{
			makeBackupName("3.1.0", 1),
			makeBackupName("3.1.0", 2),
			makeBackupName("3.1.0", 3), // keep the highest rev
		},
		leftFiles: []string{makeBackupName("3.1.0", 3)},
	}, {
		maxFiles: 2,
		files: []string{
			makeBackupName("3.1.0", 1),
			makeBackupName("3.1.0", 2),
			makeBackupName("3.1.0", 3), // keep two of the highest revs
		},
		leftFiles: []string{makeBackupName("3.1.0", 2), makeBackupName("3.1.0", 3)},
	}}

	for i, tt := range tests {
		dir, err := ioutil.TempDir("", "etcd-operator-test")
		if err != nil {
			t.Fatal(err)
		}
		fb := &fileBackend{dir}
		for _, name := range tt.files {
			err := ioutil.WriteFile(filepath.Join(dir, name), []byte("ignore"), 0600)
			if err != nil {
				t.Fatal(err)
			}
		}
		fb.purge(tt.maxFiles)
		infos, err := ioutil.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}

		var names []string
		for _, f := range infos {
			names = append(names, f.Name())
		}
		if !reflect.DeepEqual(tt.leftFiles, names) {
			t.Errorf("#%d: left files, want=%v, get=%v", i, tt.leftFiles, names)
		}

		if err := os.RemoveAll(dir); err != nil {
			t.Logf("can't remove dir (%s)", dir)
		}
	}
}
