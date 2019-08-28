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

package util

import (
	"fmt"
	"reflect"
	"testing"
)

// Tests that SortableBackupPaths.Len() simply delegates the call to the slice.
func TestSortableBackupPathsLen(t *testing.T) {
	tests := []struct {
		backupPaths []string
	}{
		// empty list
		{
			backupPaths: []string{},
		},
		// single entry
		{
			backupPaths: []string{"path"},
		},
		// multiple entries
		{
			backupPaths: []string{"path1", "path2", "path3"},
		},
	}

	for i, tt := range tests {
		expected := len(tt.backupPaths)
		actual := SortableBackupPaths(tt.backupPaths).Len()
		if actual != expected {
			t.Errorf("#%d: SortableBackupPaths.Len failed.  Expected %d, but got %d", i, expected, actual)
		}
	}
}

// Tests that SortableBackupPaths.Less() correctly determines the age ordering of etcd backup paths.
func TestSortableBackupPathsLess(t *testing.T) {
	// for all test cases, we expect the first element to be less/older than the second
	tests := []struct {
		backupPaths  []string
		lesserIndex  int
		greaterIndex int
		expectPanic  bool
	}{
		// etcd in normal use - timestamp and revision increase
		{
			backupPaths: []string{
				"prefix_v10_2020-01-02-10:00:00",
				"prefix_v20_2020-01-02-11:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// mixed with other paths that are not part of the comparison
		{
			backupPaths: []string{
				"foo",
				"prefix_v10_2020-01-02-10:00:00",
				"bar",
				"prefix_v20_2020-01-02-11:00:00",
				"baz",
			},
			lesserIndex:  1,
			greaterIndex: 3,
		},
		// etcd idle - timestamp increases by a second, while revision remains the same
		{
			backupPaths: []string{
				"prefix_v1_2020-01-01-10:00:00",
				"prefix_v1_2020-01-01-10:00:01",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// greater/newer is placed before lesser/older
		{
			backupPaths: []string{
				"prefix_v1_2020-01-01-10:00:01",
				"prefix_v1_2020-01-01-10:00:00",
			},
			lesserIndex:  1,
			greaterIndex: 0,
		},
		// index out of bounds - i
		{
			backupPaths: []string{
				"prefix_v1_2020-01-01-10:00:00",
				"prefix_v1_2020-01-01-10:00:01",
			},
			lesserIndex:  2,
			greaterIndex: 1,
			expectPanic:  true,
		},
		// index out of bounds - j
		{
			backupPaths: []string{
				"prefix_v1_2020-01-01-10:00:00",
				"prefix_v1_2020-01-01-10:00:01",
			},
			lesserIndex:  2,
			greaterIndex: 1,
			expectPanic:  true,
		},
		// timestamp minute takes precedence over second
		{
			backupPaths: []string{
				"prefix_v1_2020-01-01-10:00:59",
				"prefix_v1_2020-01-01-10:01:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// timestamp hour takes precedence over minute
		{
			backupPaths: []string{
				"prefix_v1_2020-01-01-10:59:00",
				"prefix_v1_2020-01-01-11:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// timestamp day takes precedence over hour
		{
			backupPaths: []string{
				"prefix_v1_2020-01-01-23:00:00",
				"prefix_v1_2020-01-02-10:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// timestamp month takes precedence over day
		{
			backupPaths: []string{
				"prefix_v1_2020-01-30-10:00:00",
				"prefix_v1_2020-02-01-10:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// timestamp year takes precedence over month
		{
			backupPaths: []string{
				"prefix_v1_2020-12-01-10:00:00",
				"prefix_v1_2021-01-01-10:00:01",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// old backup restored - timestamp increases, but revision number reverts
		{
			backupPaths: []string{
				"prefix_v100_2020-01-03-10:00:00",
				"prefix_v1_2020-01-03-11:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// ensure that only the last timestamp found is used
		{
			backupPaths: []string{
				"prefixWithUnexpectedTimestamp_2020-01-04-11:00:00-_v1000_2020-01-04-10:00:00",
				"prefixWithUnexpectedTimestamp_2020-01-04-10:00:00-_v1000_2020-01-04-11:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// different prefixes of different lengths - still sort by timestamp
		{
			backupPaths: []string{
				"zzzLongLongLongPrefix_v1111_2020-01-05-10:00:00",
				"aaaShorterPrefix_v1111_2020-01-05-11:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// the one without a timestamp in the expected format is considered older
		{
			backupPaths: []string{
				"prefix_v2222_2020-01-06-12:00",
				"prefix_v2222_2020-01-06-10:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// timestamp tie - sort by revision number
		{
			backupPaths: []string{
				"prefix_v3333-2020-01-07-10:00:00",
				"prefix_v4444_2020-01-07-10:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// timestamp tie - the last revision number found is used to break it
		{
			backupPaths: []string{
				"prefixWithUnexpectedRevisionNumber_v2222_v1111-2020-01-04-10:00:00",
				"prefixWithUnexpectedRevisionNumber_v1111_v2222_2020-01-04-10:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// timestamp tie - the one without a revision number in the expected format is considered older
		{
			backupPaths: []string{
				"prefix_r6666_2020-01-06-10:00:00",
				"prefix_v5555_2020-01-06-10:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
		// a path with a completely wrong format is considered older
		{
			backupPaths: []string{
				"prefix",
				"prefix_v1_2020-01-01-10:00:00",
			},
			lesserIndex:  0,
			greaterIndex: 1,
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("SortableBackupPaths.Less test case #%d", i), func(t *testing.T) {
			defer func() {
				r := recover()
				if !tt.expectPanic && r != nil {
					t.Errorf("#%d: SortableBackupPaths.Less panicked unexpectedly: %v", i, r)
				}
				if tt.expectPanic && r == nil {
					t.Errorf("#%d: SortableBackupPaths.Less didn't panic as expected", i)
				}
			}()
			if !SortableBackupPaths(tt.backupPaths).Less(tt.lesserIndex, tt.greaterIndex) {
				t.Errorf("#%d: SortableBackupPaths.Less failed.  Expected %s to be lesser than %s",
					i, tt.backupPaths[tt.lesserIndex], tt.backupPaths[tt.greaterIndex])
			}
			// try again with the order swapped - returns false
			if SortableBackupPaths(tt.backupPaths).Less(tt.greaterIndex, tt.lesserIndex) {
				t.Errorf("#%d: SortableBackupPaths.Less failed.  Expected %s to be lesser than %s",
					i, tt.backupPaths[tt.lesserIndex], tt.backupPaths[tt.greaterIndex])
			}
		})
	}
}

// Tests SortableBackupPaths.Swap().
func TestSortableBackupPathsSwap(t *testing.T) {
	tests := []struct {
		backupPaths []string
		i           int
		j           int
		// only one of these needs to be set
		expectResult []string
		expectPanic  bool
	}{
		// multiple entries
		{
			backupPaths:  []string{"path1", "path2", "path3"},
			i:            0,
			j:            2,
			expectResult: []string{"path3", "path2", "path1"},
		},
		// multiple entries, noop
		{
			backupPaths:  []string{"path1", "path2", "path3"},
			i:            2,
			j:            2,
			expectResult: []string{"path1", "path2", "path3"},
		},
		// multiple entries, index out of bounds - i
		{
			backupPaths: []string{"path1", "path2", "path3"},
			i:           3,
			j:           1,
			expectPanic: true,
		},
		// multiple entries, index out of bounds - j
		{
			backupPaths: []string{"path1", "path2", "path3"},
			i:           0,
			j:           3,
			expectPanic: true,
		},
		// empty list, index out of bounds
		{
			backupPaths: []string{},
			i:           0,
			j:           0,
			expectPanic: true,
		},
		// single entry, index out of bounds - i
		{
			backupPaths: []string{"path"},
			i:           1,
			j:           0,
			expectPanic: true,
		},
		// single entry, index out of bounds - j
		{
			backupPaths: []string{"path"},
			i:           0,
			j:           1,
			expectPanic: true,
		},
		// single entry, noop
		{
			backupPaths:  []string{"path"},
			i:            0,
			j:            0,
			expectResult: []string{"path"},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("SortableBackupPaths.Swap test case #%d", i), func(t *testing.T) {
			defer func() {
				r := recover()
				if !tt.expectPanic && r != nil {
					t.Errorf("#%d: SortableBackupPaths.Swap panicked unexpectedly: %v", i, r)
				}
				if tt.expectPanic && r == nil {
					t.Errorf("#%d: SortableBackupPaths.Swap didn't panic as expected", i)
				}
			}()
			SortableBackupPaths(tt.backupPaths).Swap(tt.i, tt.j)
			if !reflect.DeepEqual(tt.backupPaths, tt.expectResult) {
				t.Errorf("#%d: SortableBackupPaths.Swap failed.  Expected %v, but got %v",
					i, tt.expectResult, tt.backupPaths)
			}
		})
	}
}
