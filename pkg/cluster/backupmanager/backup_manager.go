package backupmanager

import "k8s.io/kubernetes/pkg/api"

// BackupManager is a common abstraction used by cluster layer to handle backup related stuff.
type BackupManager interface {
	// Setup will do some setup before starting the backup pod.
	// It takes place on non-restore path.
	Setup() error
	// Clone will try to clone the previous (from) data so that it will use them
	// to creae a new cluster later.
	// It takes place on restore path.
	Clone(from string) error
	// Cleanup will cleanup any backup related stuff, e.g. backup RS, service, etc.
	Cleanup() error
	// PodSpecWithStorage sets the storage related stuff in PodSpec.
	PodSpecWithStorage(*api.PodSpec) *api.PodSpec
}
