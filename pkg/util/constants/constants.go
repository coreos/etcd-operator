package constants

import "time"

const (
	DefaultDialTimeout      = 5 * time.Second
	DefaultRequestTimeout   = 5 * time.Second
	DefaultSnapshotInterval = 1800 * time.Second

	BackupDir = "/home/backup/"
)
