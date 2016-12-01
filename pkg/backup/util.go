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
	"fmt"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
)

func getLatestBackupName(names []string) string {
	maxRev := int64(0)
	name := ""
	for _, n := range names {
		if !isBackup(n) {
			continue
		}
		rev, err := getRev(n)
		if err != nil {
			logrus.Errorf("fail to get rev from backup (%s): %v", n, err)
			continue
		}
		if rev > maxRev {
			maxRev = rev
			name = n
		}
	}
	return name
}

func isBackup(name string) bool {
	return strings.HasSuffix(name, backupFilenameSuffix)
}

func makeBackupName(ver string, rev int64) string {
	return fmt.Sprintf("%s_%016x_%s", ver, rev, backupFilenameSuffix)
}

func getRev(name string) (int64, error) {
	return strconv.ParseInt(strings.SplitN(name, "_", 3)[1], 16, 64)
}
