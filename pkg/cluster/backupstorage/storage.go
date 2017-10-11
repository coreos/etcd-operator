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

package backupstorage

// Storage defines the underlying storage used by backup sidecar.
type Storage interface {
	// Create creates the actual persistent storage.
	// We need this method because this has side effect, e.g. creating PVC.
	// We might not create the persistent storage again when we know it already exists.
	Create() error
	// Clone will try to clone another storage referenced by cluster name.
	// It takes place on restore path.
	Clone(from string) error
	// Delete will delete this storage.
	Delete() error
}
