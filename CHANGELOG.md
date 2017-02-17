## [Unreleased 0.2.1]
### Added

- Experimental client for interacting with backup service
- The operator panics itself when it gets stuck unexpectedly. It relies on Kubernetes to
get restarted.
- Add resource requirements field in `Spec.Pod` . Users can specify resource requirements for the
etcd container with this new field.
- Add status endpoint to backup sidecar service.
- Service account of the etcd operator pod is passed to backup pod.
- Add backup service status into cluster status.

### Changed

- Example deployments pin to the released version of the operator image
- Downward API of pod's namespace and name is required to start etcd operator pod.
  See `example/deployment.yaml` .

### Removed

### Fixed

### Deprecated

### Security

