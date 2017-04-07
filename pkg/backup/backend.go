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

import "io"

type backend interface {
	// save saves the backup from the given reader with given etcd version and revision.
	// It returns the size of the snapshot saved.
	save(etcdVersion string, rev int64, r io.Reader) (size int64, err error)

	// get latest backup's name.
	// If no backup is available, returns empty string name.
	getLatest() (name string, err error)

	// open a backup file for reading
	open(name string) (rc io.ReadCloser, err error)

	// total returns the total number of available backups.
	total() (int, error)

	// total returns the total size of the backups.
	totalSize() (int64, error)

	purge(maxBackupFiles int) error
}
