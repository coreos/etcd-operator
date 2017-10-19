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

package backupapi

import (
	"fmt"
	"net/url"
	"path"
)

const (
	HTTPQueryVersionKey  = "etcdVersion"
	HTTPQueryRevisionKey = "etcdRevision"
)

// NewBackupURL creates a URL struct for retrieving an existing backup.
func NewBackupURL(scheme, host, version string, revision int64) *url.URL {
	return BackupURLForCluster(scheme, host, "", version, revision)
}

// BackupURLForCluster creates a URL struct for retrieving an existing backup of given cluster.
func BackupURLForCluster(scheme, host, clusterName, version string, revision int64) *url.URL {
	u := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path.Join(APIV1, "backup", clusterName),
	}
	uv := url.Values{}
	uv.Set(HTTPQueryVersionKey, version)
	if revision >= 0 {
		uv.Set(HTTPQueryRevisionKey, fmt.Sprintf("%d", revision))
	}
	u.RawQuery = uv.Encode()

	return u
}
