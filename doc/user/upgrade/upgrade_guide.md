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
