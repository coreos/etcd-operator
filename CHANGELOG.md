## [Unreleased 0.3.1]

### Added

- Self-hosted etcd: if `etcd-hosts.checkpoint` file exists under `${datadir}/`,
  etcd pod will restore the hosts mapping from it before etcd bootstraps.
- Add static TLS support for self-hosted etcd mode.
- The operator will now post Kubernetes [events](https://kubernetes.io/docs/resources-reference/v1.6/#event-v1-core). To allow this the necessary RBAC rule for the resource `events` must be added to the clusterrole. See the [rbac guide](https://github.com/coreos/etcd-operator/blob/master/doc/user/rbac.md#create-clusterrole) to see how to set up RBAC rules for the operator. If the rbac rule for 'events' is not present then the operator will continue to function normally but will also print out an error message on the failure to post an event.
- Add revision field in backup status.
- Support getting a specific backup with verison and revision from the backup service.

### Changed

- Self-hosted etcd: use FQDN for client/peer URL.
- Updated RBAC rules for resources `thirdpartyresources` and `storageclasses` to all verbs `*` . We loose granularity early so that we have more flexibility to use other methods (e.g. Get) later.

### Removed

- Update default etcd version to 3.1.8

### Fixed

- [GH-1108] selfHosted: fix backup unable to talk to etcd pods

### Deprecated

### Security


## [Release 0.3.0]

### Upgrade Notice

Check https://github.com/coreos/etcd-operator/blob/master/doc/user/upgrade/upgrade_guide.md#v02x-to-v03x

### Added

- Added support for backup policy to be dynamically added, updated
- Added per cluster policy support for S3.

### Changed

- Backup sidecar deployment created with `Recreate` strategy.
- Spec.Backup.MaxBackups meaning change: 0 means unlimited backups; < 0 will be rejected.

### Removed

### Fixed

- [GH-1068] Backup sidecar deployment stuck at upgrading

### Deprecated

### Security


## [Release 0.2.6]

### Upgrade Notice

- Once operator is upgraded, all backup-enabled cluster will go through an upgrade process that
  deletes backup sidecar's ReplicaSet and creates new Deployment for sidecar.
  If upgrading failed for any reason, cluster TPR's `status.phase` will be FAILED.
  Recreate of the cluster TPR is needed on failure case.

### Added

- PodPolicy provides `EtcdEnv` option to add custom env to the etcd process.
- PodPolicy provides `Labels` option to add custom labels to the etcd pod.
- TLS feature: user can now create TLS-secured cluster via operator.
  See [TLS guide](https://github.com/coreos/etcd-operator/blob/master/doc/user/cluster_tls.md).

### Changed

- Self-hosted etcd pod's anti-affinity label selector is changed to select `{"app": "etcd"}`.
  That is, no two etcd pods should sit on the same node, even if they belongs to different clusters.
- Using Deployment to manage backup sidecar instead of ReplicaSet.
- S3 backup path is changed to `${BUCKET_NAME}/v1/${NAMESPACE}/${CLUSTER_NAME}/`.

### Removed

### Fixed

### Deprecated

### Security


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

