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

package backupapi

import "time"

type ServiceStatus struct {
	// RecentBackup is status of the most recent backup created by
	// the backup service
	RecentBackup *BackupStatus `json:"recentBackup,omitempty"`

	// Backups is the totoal number of existing backups.
	Backups int `json:"backups"`

	// BackupSize is the total size of existing backups in MB.
	BackupSize float64 `json:"backupSize"`

	// StorageStatus is the status of the storage the backup pod uses.
	StorageStatus *StorageStatus `json:"storageStatus,omitempty"`
}

type BackupStatus struct {
	// Creation time of the backup.
	CreationTime time.Time `json:"creationTime"`

	// Size is the size of the backup in MB.
	Size float64 `json:"size"`

	// Version is the version of the backup cluster.
	Version string `json:"version"`

	// TimeTookInSecond is the total time took to create the backup.
	TimeTookInSecond int `json:"timeTookInSecond"`
}

type StorageStatus struct {
	// PV is the status of Persist Volume
	PV *PVStatus `json:"pv,omitempty"`
}

type PVStatus struct {
	// Name is the name of the Persist Volume
	Name string `json:"name"`
}
