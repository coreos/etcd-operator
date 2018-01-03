## [Unreleased]

### Added

- Add readiness probe to etcd pod. The readiness state will be reflected on `status.members.ready/unready`.

### Changed

- Default base image is changed to `gcr.io/etcd-development/etcd`, default etcd version is `3.2.13`.

### Removed

### Fixed

### Deprecated

### Security

## [Release 0.7.2]

> Note: This is a bug fix release.

When we bump the etcd version to 3.2, the images were only available on gcr.io . But now it is added on quay.io .
We'd better use quay.io and keep it compatible to work for 3.1 versions of etcd.

## [Release 0.7.1]

### Added

- TLS etcd cluster feature for EtcdBackup
- Log collector program for collecting logs in e2e test.
- ClusterSpec: In PodPolicy, add generic `Affinity` field to substitute bool `AntiAffinity` field.
- ClusterSpec: Add `Repository` field to substitute `BaseImage` field.

### Changed

- Default base image is changed to `gcr.io/etcd-development/etcd`, default etcd version is `3.2.11`.
- Migrate dependency management tooling from glide to dep.
- Containerize e2e test in a pod instead of running on raw jenkin slave.

### Removed

- ClusterSpec: Remove `PodPolicy.AutomountServiceAccountToken` field.
  No etcd pod will have service account token mounted.

### Fixed

- Ignore Terminating pods when polling etcd pods.

### Deprecated

- ClusterSpec: `BaseImage` is deprecated. It will be automatically converted to `Repository` in this release.
- ClusterSpec: In PodPolicy, `AntiAffinity` is deprecated. It will be automatically converted to `Affinity.PodAntiAffinity`
  terms with label selector on given cluster name and topology key on node in this release.

### Security

- All operator images by default uses user `etcd-operator` instead of root.


## [Release 0.7.0]

Existing backup and restore features in EtcdCluster API won’t be supported after 0.7.0 release. 
See [Decoupling Backup and Restore Logic from Etcd Operator](https://github.com/coreos/etcd-operator/issues/1626) for more detail.

If applicable then see the [upgrade guide](./doc/user/upgrade/upgrade_guide.md) on how to upgrade from `v0.6.1` to `v0.7.0` .

### Added

- Add `ServiceName` and `ClientPort` into ClusterStatus.
- Add etcd backup operator for backing up an etcd cluster.
- Add etcd restore operator for restoring an etcd cluster.

### Removed

- Remove `pv-provisioner` flag from etcd operator.
- Remove etcd cluster Backup feature from etcd operator.
- Remove etcd cluster Restore from etcd operator.


## [Release 0.6.1]

The operator will no longer create a storage class specified by `--pv-provisioner` by default. If applicable then see the [upgrade guide](./doc/user/upgrade/upgrade_guide.md) on how to upgrade from `v0.6.0` to `v0.6.1` .

### Added

- backup binary supports serving backup defined by backupSpec. In addition, when backupSpec
is specified, backup binary changes to serve http backup requests only mode.
- Add operator flag `--create-crd`. By default it is `true` and operator will create EtcdCluster CRD.
  It can be set to `false` and operator won't create EtcdCluster CRD.
- Add operator flag `--create-storage-class`. By default it is `false` and operator won't create default storage class.
  It can be set to `true` and operator will create default storage class.

### Changed

- An EtcdCluster CR with an invalid spec will not be marked as failed. Any changes that result in an invalid spec will be ignored and logged by the operator.

### Fixed

- Fix the problem that operator might keep failing on version conflict updating CR status.

### Deprecated

- The operator flag `--pv-provisioner` is depercated. We recommend to use per cluster storageClass.


## [Release 0.6.0]

**BREAKING CHANGE**: operator level S3 backup is removed. See [upgrade](./doc/user/upgrade/upgrade_guide.md) on how to upgrade from 0.5.x to 0.6.0. 

### Added

- Add cluster events into EtcdCluster custom resource. See `doc/user/conditions_and_events.md` .

### Changed

- Redefine status.conditions. See `doc/user/conditions_and_events.md` .

### Removed

- Remove operator level S3 flag.
- Remove analytics flag. Disable Google analytics.


## [Release 0.5.2]

### Added

- Expose `/metrics` endpoint at port 8080
- Add cluster S3 spec `prefix` feature. Let user choose a prefix under the bucket.
- Add `automountServiceAccountToken` to pod policy. Let users disable automounting of the Kubernetes access token into etcd-operator controlled pods.
- Cluster backups can now be saved using Azure Blob Storage (ABS).

### Deprecated

- Deprecate operator S3 flag. Add warning note for using it in this release. The flag will be removed in 0.6.0 release.


## [Release 0.5.1]

Upgrade notice for TLS cluster users:
If you are using TLS-enabled etcd cluster, the SAN domain has been changed. See [TLS docs](./doc/user/cluster_tls.md).
Before upgrading operator, you need to rotate certs on each secrets to allow both the old and new domains.
Then restart each etcd pod -- the simplest way is to "upgrade" cluster version.
Finally, it is safe to upgrade operator. It's highly recommended to save a backup before upgrade.

### Added

- A new `StorageClass` spec field, allowing more granular control over how etcd clusters are backed up to PVs.

### Changed

- Default timeout for snapshots done by backup sidecar increased from 5 seconds to 1 minute

### Fixed

- Fix periodFullGC only executed once problem.
- [GH-1021] Use the cluster domain provided by kubelet instead of hardcoded `.cluster.local` .


## [Release 0.5.0]

**BREAKING CHANGE**: The cluster object will now be defined via a [Custom Resource Definition(CRD)](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/) instead of a [Third Party Resource(TPR)](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-third-party-resource/). See the `Changed` section below for details.

### Changed

- With k8s 1.7 and onwards TPRs have been deprecated and are replaced with CRD. See the k8s 1.7 [blogpost](http://blog.kubernetes.io/2017/06/kubernetes-1.7-security-hardening-stateful-application-extensibility-updates.html) or [release notes](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG.md#major-themes) for more details. For this release a live migration of the cluster spec from TPR to CRD is not supported. To preserve the cluster state during the upgrade you will need to create a backup of the cluster and recreate the cluster from the backup after upgrading the operator. See the [upgrade guide](https://github.com/coreos/etcd-operator/blob/master/doc/user/upgrade/upgrade_guide.md) for more detailed steps on how to do that.

- Changes in the cluster object's type metadata:
  - The `apiVersion` field has been changed from `etcd.coreos.com/v1beta1` to `etcd.database.coreos.com/v1beta2`
  - The `kind` field has been changed from `Cluster` to `EtcdCluster`


## [Release 0.4.2]

### Added

- [GH-1232](https://github.com/coreos/etcd-operator/pull/1232) the operator can now log critical actions like pod creation/deletion to a user specified path via the optional flag `debug-logfile-path`. The logs will only be generated if the cluster is self hosted and the flag is set. This can be used in conjunction with a persistent volume to persist the critical actions to disk for later inspection.

### Changed

- enable alpha feature "tolerate unready endpoints" on etcd client and peer service

### Fixed

- Fix append-hosts init-container not run on some restart cases.


## [Release 0.4.1]

This is a bug-fix release. We have done a lot of testing against k8s 1.7
and making it stable on 1.7 .

### Added

- New self-hosted field `SkipBootMemberRemoval` allows users to skip the
  auto-deletion of the boot etcd member.

### Fixed

- Make sure etcd pod's FQDN is resolvable before running etcd commands .


## [Release 0.4.0]

**BREAKING CHANGE**: Re-naming of TLS spec and TLS secrets' fields.

TLS spec:
- member's `clientSecret` is changed to `serverSecret`

TLS secrets:
- member's `peerSecret`'s fields change:
  - peer-crt.pem -> peer.crt
  - peer-key.pem -> peer.key
  - peer-ca-crt.pem -> peer-ca.crt
- member's `clientSecret` is changed to `serverSecret`, its fields change:
  - client-crt.pem -> server.crt
  - client-key.pem -> server.key
  - client-ca-crt.pem -> server-ca.crt
- `operatorSecret`'s fields change:
  - etcd-crt.pem -> etcd-client.crt
  - etcd-key.pem -> etcd-client.key
  - etcd-ca-crt.pem -> etcd-client-ca.crt


**BREAKING CHANGE**: Backup spec: `CleanupBackupsOnClusterDelete` field is renamed to `AutoDelete`.

Previous spec like this one

```yaml
spec:
  backup:
    storageType: "PersistentVolume"
    ...
    cleanupBackupsOnClusterDelete: true
```

should be changed to

```yaml
spec:
  backup:
    storageType: "PersistentVolume"
    ...
    autoDelete: true
```


## [Release 0.3.3]

### Added

- Adds ability for users to specify base image for etcd pods in a cluster.
  Default base image is `quay.io/coreos/etcd-operator`.

### Fixed

- [GH-1138] Fixed operator stucks in managing selfhosted cluster when there are not enough nodes to start new etcd member.
- [GH-1196] Fixed etcd operator could not start S3 backup sidecar if given non-root user.


## [Release 0.3.2]

Bug fix release to fix self-hosted etcd issue [GH-1171] .

## [Release 0.3.1]


**Notes for self-hosted etcd**:
The newly introduced TLS feature for self hosted etcd is a breaking change.
Existing self hosted etcd cluster MUST be recreated for updating to this release.

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
