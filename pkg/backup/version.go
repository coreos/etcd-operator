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
	"strings"

	"github.com/coreos/go-semver/semver"
)

var compatibilityMap = map[string]map[string]struct{}{
	"3.0": {"2.3": struct{}{}, "3.0": struct{}{}},
	"3.1": {"3.0": struct{}{}, "3.1": struct{}{}},
}

func getVersionFromBackupName(name string) (string, error) {
	return getMajorAndMinorVersion(strings.SplitN(name, "_", 2)[0])
}

func isVersionCompatible(req, serve string) bool {
	compatVersions, ok := compatibilityMap[req]
	if !ok {
		return false
	}
	_, ok = compatVersions[serve]
	return ok
}

// getMajorAndMinorVersion expects a semver and then returns "major.minor"
func getMajorAndMinorVersion(rawV string) (string, error) {
	v, err := semver.NewVersion(rawV)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor), nil
}
