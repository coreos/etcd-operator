## [Unreleased 0.2.3]

### Added

### Changed

### Removed

### Fixed

- [GH-890] Fix a race that when majority of members went down cluster couldn't recover

### Deprecated

### Security


## [Release 0.2.2]

### Added

- Backup creation time is added in backup status.
- Total size of backups time is added in backup service status.
- Cluster members that are ready and unready to serve requests are tracked via the ClusterStatus fields `Members.Ready` and `Members.Unready`

### Changed

- PodPolicy `resourceRequirements` field is renamed to `resources`
- Default etcd version is changed to `3.1.2`
- Self-hosted etcd pod uses hostPath with path `/var/etcd/$ns-$member`

### Removed

### Fixed

- [GH-851] Fixed a race that caused nil pointer access panic
- [GH-823] Fixed backup service status not shown in TPR status

### Deprecated

### Security


## [Release 0.2.1]
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

- Drop etcd operator command line flags: "masterHost", "cert-file", "key-file", "ca-file".

### Fixed

### Deprecated

### Security

