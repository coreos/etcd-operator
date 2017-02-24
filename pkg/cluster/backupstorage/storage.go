package backupstorage

// Storage defines the underlying storage used by backup sidecar.
type Storage interface {
	// Create creates the actual persistent storage. It returns the unique identifier
	// of the storage created as a string.
	// We need this method because this has side effect, e.g. creating PVC.
	// We might not create the persistent storage again when we know it already exists.
	Create() (string, error)
	// Clone will try to clone another storage referenced by cluster name.
	// It takes place on restore path.
	Clone(from string) error
	// Delete will delete this storage.
	Delete() error
}
