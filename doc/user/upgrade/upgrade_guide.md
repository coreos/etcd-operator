# Upgrade Guide

This document shows how to safely upgrade the operator to a desired version while preserving the cluster's state and data whenever possible. It is assumed that the preexisting cluster is configured to create and store backups to persistent storage. See the [backup config guide](../backup_config.md) for details.

### Backup safety precaution:
First create a backup of your current cluster before starting the upgrade process. See the [backup service guide](../backup_service.md) on how to create a backup.

In the case of an upgrade failure you can restore your cluster to the previous state from the previous backup. See the [spec examples](https://github.com/coreos/etcd-operator/blob/master/doc/user/spec_examples.md#three-members-cluster-that-restores-from-previous-pv-backup) on how to do that.


### Upgrade operator deployment
An [in-place update](https://kubernetes.io/docs/concepts/cluster-administration/manage-deployment/#in-place-updates-of-resources) can be performed when the upgrade is compatible, i.e we can upgrade the operator without affecting the cluster.

To upgrade an operator deployment the image field `spec.template.spec.containers.image` needs to be changed via an in-place update.

Change the image field to `quay.io/coreos/etcd-operator:vX.Y.Z`  where `vX.Y.Z` is the desired version.
```bash
$ kubectl edit deployment/etcd-operator
# make the image change in your editor then save and close the file
```

### Incompatible upgrade
In the case of an incompatible upgrade, the process requires restoring a new cluster from backup. See the [incompatible upgrade guide](incompatible_upgrade.md) for more information.

## v0.4.x -> v0.5.x
For any `0.4.x` versions, please update to `0.5.0` first.

The `0.5.0` release introduces a breaking change in moving from [TPR](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-third-party-resource/) to [CRD](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/). To preserve the cluster state across the upgrade, the cluster must be recreated from backup after the upgrade.
### Prerequisite:
Kubernetes cluster version must be 1.7+.

### Steps to upgrade:
- Create a backup of your current cluster data. See the following guides on how to enable and create backups:
    - [backup config guide](https://github.com/coreos/etcd-operator/blob/master/doc/user/backup_config.md) to enable backups fro your cluster
    - [backup service guide](https://github.com/coreos/etcd-operator/blob/master/doc/user/backup_service.md) to create a backup
- Delete the cluster TPR object. The etcd-operator will delete all resources(pods, services, deployments) associated with the cluster:
    - `kubectl -n <namespace> delete cluster <cluster-name>`
- Delete the etcd-operator deployment
- Delete the TPR
    - `kubectl delete thirdpartyresource clusters.etcd.coreos.com `
- Replace the existing RBAC rules for the etcd-operator by editing or recreating the `ClusterRole` with the [new rules for CRD](https://github.com/coreos/etcd-operator/blob/master/doc/user/rbac.md#create-clusterrole).
- Recreate the etcd-operator deployment with the `0.5.0` image.
- Create a new cluster that restores from the backup of the previous cluster. The new cluster CR spec should look as follows:
```
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdCluster"
metadata:
  name: <cluster-name>
spec:
  size: <cluster-size>
  version: "3.1.8"
  backup:
    # The same backup spec used to save the backup of the previous cluster
    . . .
    . . .
  restore:
    backupClusterName: <previous-cluster-name>
    storageType: <storage-type-of-backup-spec>
```
The two points of interest in the above CR spec are:
  1. The `apiVersion` and `kind` fields have been changed as mentioned in the `0.5.0` release notes
  2. The `spec.restore` field needs to be specified according your backup configuration. See [spec examples guide](https://github.com/coreos/etcd-operator/blob/master/doc/user/spec_examples.md#three-members-cluster-that-restores-from-previous-pv-backup) on how to specify the `spec.restore` field for your particular backup configuration.

## v0.3.x -> v0.4.x
Upgrade to `v0.4.0` first.
See the release notes of `v0.4.0` for noticeable changes: https://github.com/coreos/etcd-operator/releases/tag/v0.4.0

## v0.2.x -> v0.3.x
### Prerequisite:
To upgrade to `v0.3.x` the current operator verison **must** first be upgraded to `v0.2.6+`, since versions < `v0.2.6` are not compatible with `v0.3.x` .

### Noticeable changes:
- `Spec.Backup.MaxBackups` update:
  - If you have `Spec.Backup.MaxBackups < 0`, previously it had no effect.
    Now it will get rejected. Please remove it.
  - If you have `Spec.Backup.MaxBackups == 0`, previously it had no effect.
    Now it will create backup sidecar that allows unlimited backups.
    If you don't want backup sidecar, just don't set any backup policy.
