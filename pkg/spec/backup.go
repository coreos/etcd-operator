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

package spec

import "time"

type BackupStorageType string

const (
	BackupStorageTypeDefault          = ""
	BackupStorageTypePersistentVolume = "PersistentVolume"
	BackupStorageTypeS3               = "S3"
)

type BackupPolicy struct {
	// StorageType specifies the type of storage device to store backup files.
	// If it's not set by user, the default is "PersistentVolume".
	StorageType BackupStorageType `json:"storageType"`

	StorageSource `json:",inline"`

	// BackupIntervalInSecond specifies the interval between two backups.
	// The default interval is 1800 seconds.
	BackupIntervalInSecond int `json:"backupIntervalInSecond"`

	// MaxBackups is the maximum number of backup files to retain. 0 is disable backup.
	// If backup is disabled, the etcd cluster cannot recover from a
	// disaster failure (lose more than half of its members at the same
	// time).
	MaxBackups int `json:"maxBackups"`

	// CleanupBackupsOnClusterDelete tells whether to cleanup backup data if cluster is deleted.
	// By default, operator will keep the backup data.
	CleanupBackupsOnClusterDelete bool `json:"cleanupBackupsOnClusterDelete"`
}

type StorageSource struct {
	PV *PVSource `json:"pv,omitempty"`
	S3 *S3Source `json:"s3,omitempty"`
}

type PVSource struct {
	// VolumeSizeInMB specifies the required volume size to perform backups.
	// Operator will claim the required size before creating the etcd cluster for backup
	// purpose.
	// If the snapshot size is larger than the size specified, backup fails.
	VolumeSizeInMB int `json:"volumeSizeInMB"`
}

type S3Source struct {
}

type BackupServiceStatus struct {
	// RecentBackup is status of the most recent backup created by
	// the backup service
	RecentBackup *BackupStatus `json:"recentBackup,omitempty"`

	// Backups is the totoal number of existing backups
	Backups int `json:"backups"`

	// BackupSize is the total size of existing backups in MB.
	BackupSize float64 `json:"backupSize"`
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
