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
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func MakeBackupName(ver string, rev int64) string {
	return fmt.Sprintf("%s_%016x_%s", ver, rev, BackupFilenameSuffix)
}

// ParseBucketAndKey parses the path to return the s3 bucket name and key(path in the bucket)
// returns error if path is not in the format <s3-bucket-name>/<key>
func ParseBucketAndKey(path string) (string, string, error) {
	toks := strings.SplitN(path, "/", 2)
	if len(toks) != 2 || len(toks[0]) == 0 || len(toks[1]) == 0 {
		return "", "", fmt.Errorf("Invalid S3 path (%v)", path)
	}
	return toks[0], toks[1], nil
}

// SortableBackupPaths implements extends sort.StringSlice to allow sorting to work
// with paths used for backups, in the format "<base path>_v<etcd store revision>_YYYY-MM-DD-HH:mm:SS",
// where the timestamp is what is being sorted on.
type SortableBackupPaths sort.StringSlice

// regular expressions used in backup path order comparison
var backupTimestampRegex = regexp.MustCompile(`_\d+-\d+-\d+-\d+:\d+:\d+`)
var etcdStoreRevisionRegex = regexp.MustCompile(`_v\d+`)

// Len is the number of elements in the collection.
func (s SortableBackupPaths) Len() int {
	return len(s)
}

// Less reports whether the element with index i should sort before the element with index j.
// Assumes that the two paths are part of a sequence of backups of the same etcd cluster,
// relying on comparison of the timestamps found in the two elements' paths,
// but not making assumptions about either path's base path.
// The last etcd store revision number found in each path is used to break ties.
// Timestamp comparison takes precedence in case an older revision of the etcd store is restored
// to the cluster, resulting in newer, more relevant backups that happen to have older revision numbers.
// If either base path happens to have a similarly formatted revision number,
// the last one in each path is compared.
// This method will work even if the format changes somewhat (e.g., the revision is placed after the timesamp).
// If a comparison can't be made based on path format, the one conforming to the format is considered to be newer,
// and if neither path conforms, false is returned.
func (s SortableBackupPaths) Less(i, j int) bool {
	// compare timestamps first
	itmatches := backupTimestampRegex.FindAll([]byte(s[i]), -1)
	jtmatches := backupTimestampRegex.FindAll([]byte(s[j]), -1)

	if len(itmatches) < 1 {
		return true
	}
	if len(jtmatches) < 1 {
		return false
	}

	itstr := string(itmatches[len(itmatches)-1])
	jtstr := string(jtmatches[len(jtmatches)-1])

	timestampComparison := strings.Compare(string(jtstr), string(itstr))
	if timestampComparison != 0 {
		return timestampComparison > 0
	}

	// fall through to revision comparison
	irmatches := etcdStoreRevisionRegex.FindAll([]byte(s[i]), -1)
	jrmatches := etcdStoreRevisionRegex.FindAll([]byte(s[j]), -1)

	if len(irmatches) < 1 {
		return true
	}
	if len(jrmatches) < 1 {
		return false
	}

	irstr := string(irmatches[len(irmatches)-1])
	jrstr := string(jrmatches[len(jrmatches)-1])

	ir, _ := strconv.ParseInt(irstr[2:len(irstr)], 10, 64)
	jr, _ := strconv.ParseInt(jrstr[2:len(jrstr)], 10, 64)

	return ir < jr
}

// Swap swaps the elements with indexes i and j.
func (s SortableBackupPaths) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
