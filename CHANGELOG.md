## [Unreleased 0.2.6]

## [Release 0.2.5]

### Added

- Add "none" PV provisioner option. If operator flag "pv-provisioner" is set to "none",
  operator won’t create any storage class and users couldn’t make use of operator’s PV backup feature.
- Add headless service `${clusterName}` which selects etcd pods of given cluster.
- Pod Tolerations.

### Changed

- TLSSpec json tag changed as `omitempty`
- Time related fields in spec, i.e. TransitionTime and CreationTime, is changed to type `string`.
  This should be backward compatible and no effect on operator upgrade.
- Update default etcd version to 3.1.4
- Self-hosted etcd pod is started with "--metrics extensive" flag.
  This is only available in etcd 3.1+.
- Change client LB service name to `${clusterName}-client`.
- Add hostname and subdomain to etcd pods, which makes them have A records formatted in `${memberName}.${clusterName}.${namespace}.svc.cluster.local` .
  For more info, see https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/ .
  We also change PeerURL of etcd members to use such hostnames.

### Removed

- Individual etcd member's services were removed. Use hostname and subdomain of etcd pod instead.

### Fixed

- [GH-910] Operator keeps updating status even if there is no change.

### Deprecated

### Security


## [Release 0.2.4]

### Added

### Changed

### Removed

### Fixed

- [GH-900] Fix looping of reconcile skip due to unready members

### Deprecated

### Security


## [Release 0.2.3]

### Added

### Changed

### Removed

### Fixed

- [GH-890] Fix a race that when majority of members went down cluster couldn't recover
- Fix self-hosted cluster reboot case

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

