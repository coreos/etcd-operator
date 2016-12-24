package backupstorage

// Storage defines the underlying storage used by backup sidecar.
type Storage interface {
	// Clone will try to clone another storage referenced by cluster name.
	// It takes place on restore path.
	Clone(from string) error
	// Delete will delete this storage.
	Delete() error
}
