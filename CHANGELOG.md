## [Unreleased 0.2.1]
### Added

- Experimental client for interacting with backup service
- The operator panics itself when it gets stuck unexpectedly. It relies on Kubernetes to
get restarted.
- Add resource requirements field in Spec.Pod. Users can specify resource requirements for the
etcd container with this new field.

### Changed

- Example deployments pin to the released version of the operator image

### Removed

### Fixed

### Deprecated

### Security

