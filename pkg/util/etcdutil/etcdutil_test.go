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

package etcdutil

import "testing"

func TestMemberNameFromPeerURL(t *testing.T) {
	tests := []struct {
		purl  string
		wName string
		wErr  bool
	}{{
		purl:  "http://test-cluster:2379",
		wName: "test-cluster",
	}, {
		purl:  "https://test-cluster:2379",
		wName: "test-cluster",
	}, {
		purl:  "http://test-cluster",
		wName: "test-cluster",
	}, {
		purl: "test-cluster",
		wErr: true,
	}, {
		purl: "test-cluster:2379",
		wErr: true,
	}, {
		purl: "",
		wErr: true,
	}, {
		purl: "http://",
		wErr: true,
	}, {
		purl: "http://a:a",
		wErr: true,
	}}

	for i, tt := range tests {
		get, err := MemberNameFromPeerURL(tt.purl)
		if tt.wErr {
			if err == nil {
				t.Errorf("#%d: should be error case", i)
			}
			continue
		}
		if err != nil {
			t.Fatalf("#%d: want err = nil, got %v", i, err)
		}
		if get != tt.wName {
			t.Errorf("#%d: member name get=%s, want=%s", i, get, tt.wName)
		}
	}
}
