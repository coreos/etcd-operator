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

package backend

import "io"

// Backend defines required backend operations
type Backend interface {
	// Save saves the backup from the given reader with given etcd version and revision.
	// It returns the size of the snapshot saved.
	Save(etcdVersion string, rev int64, r io.Reader) (size int64, err error)

	// GetLatest gets latest backup's name.
	// If no backup is available, returns empty string name.
	GetLatest() (name string, err error)

	// Open opens a backup file for reading
	Open(name string) (rc io.ReadCloser, err error)

	// Total returns the total number of available backups.
	Total() (int, error)

	// TotalSize returns the total size of the backups.
	TotalSize() (int64, error)

	// Purge purges backup files when backups are greater than maxBackupFiles.
	Purge(maxBackupFiles int) error
}
