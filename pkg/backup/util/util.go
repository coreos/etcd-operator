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
