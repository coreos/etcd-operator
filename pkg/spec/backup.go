package spec

// TODO: supports object store like s3
type BackupStorageType string

const (
	BackupStorageTypePersistentVolume = "PersistentVolume"
)

type BackupPolicy struct {
	// SnapshotIntervalInSecond specifies the interval between two snapshots.
	// The default interval is 1800 seconds.
	SnapshotIntervalInSecond int `json:"snapshotIntervalInSecond"`
	// MaxSnapshot is the maximum number of snapshot files to retain. 0 is disable backup.
	// If backup is disabled, the etcd cluster cannot recover from a
	// disaster failure (lose more than half of its members at the same
	// time).
	MaxSnapshot int `json:"maxSnapshot"`
	// VolumeSizeInMB specifies the required volume size to perform backups.
	// Controller will claim the required size before creating the etcd cluster for backup
	// purpose.
	// If the snapshot size is larger than the size specified, backup fails.
	VolumeSizeInMB int `json:"volumeSizeInMB"`
	// StorageType specifies the type of storage device to store backup files.
	// If it's not set by user, the default is "PersistentVolume".
	StorageType BackupStorageType `json:"storageType,omitempty"`
}
