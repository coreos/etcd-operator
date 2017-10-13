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

package util

import (
	"reflect"
	"testing"
)

func TestFilterAndSortBackups(t *testing.T) {
	names := []string{
		MakeBackupName("3.0.1", 12),
		"3.0.1_18_etcd.tmp", // bad suffix
		MakeBackupName("3.0.1", 3),
		MakeBackupName("3.0.3", 19),
		MakeBackupName("3.0.0", 1),
		"3.0.1_badbackup_etcd.backup", //bad backup name
	}

	w := []string{
		MakeBackupName("3.0.0", 1),
		MakeBackupName("3.0.1", 3),
		MakeBackupName("3.0.1", 12),
		MakeBackupName("3.0.3", 19),
	}

	got := FilterAndSortBackups(names)
	if !reflect.DeepEqual(got, w) {
		t.Errorf("got = %v, want %v", got, w)
	}
}

func TestParseRevision(t *testing.T) {
	tests := []struct {
		name string
		rev  int64
		werr bool
	}{
		{MakeBackupName("3.0.0", 1), 1, false},
		{MakeBackupName("3.0.0", 8), 8, false},
		{"3.0.1_badrev_etcd.backup", 0, true}, // bad rev in backup name
		{"0", 0, true},                        // bad format
	}

	for i, tt := range tests {
		rev, err := parseRevision(tt.name)
		if rev != tt.rev {
			t.Errorf("#%d: rev = %d, want %d", i, rev, tt.rev)
		}
		if err != nil && !tt.werr {
			t.Errorf("#%d: err = %v, want nil", i, err)
		}
		if err == nil && tt.werr {
			t.Errorf("#%d: err = %v, want error", i, err)
		}
	}
}

func TestGetLatestBackupName(t *testing.T) {
	names := []string{
		MakeBackupName("3.0.0", 1),
		MakeBackupName("3.0.1", 12),
		"3.0.1_18_etcd.tmp",           // bad suffix
		"3.0.1_badbackup_etcd.backup", // bad backup name
	}

	wname := MakeBackupName("3.0.1", 12)
	gname := GetLatestBackupName(names)

	if gname != wname {
		t.Errorf("name = %s, want %s", gname, wname)
	}
}
