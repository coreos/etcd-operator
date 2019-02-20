# Upgrade Guide

This document shows how to safely upgrade the operator to a desired version while preserving the cluster's state and data whenever possible. 

### Backup safety precaution:
**Note:** Only applies to when upgrading from an etcd operator with version < v0.7.0.

First create a backup of your current cluster before starting the upgrade process. See the [backup service guide](https://github.com/coreos/etcd-operator/blob/v0.6.1/doc/user/backup_service.md) on how to create a backup.

In the case of an upgrade failure you can restore your cluster to the previous state from the previous backup. See the [spec examples](https://github.com/coreos/etcd-operator/blob/v0.6.1/doc/user/spec_examples.md) on how to do that.

## v0.6.1 -> v0.7.0
**Note:** if your cluster specifies either the backup policy or restore policy, then follow the  [migrate CR](./migrate_cr_070.md) guide to update the cluster spec before upgrading the etcd-operator deployment.

Update the etcd-operator deployment:
- Edit the existing etcd-operator deployment: `kubectl edit deployment/<your-operator>`.
- Modify `image` to `quay.io/coreos/etcd-operator:v0.7.0`.
- If `command` field doesn't exist, add a new `command` field to run `etcd-operator`:

  ```
  command:
  - etcd-operator
  ```
- If `command` field exists and `pv-provisioner` flag is used, you must remove `pv-provisioner` flag.
- Save.

## v0.6.0 -> v0.6.1

In `v0.6.1+` the operator will no longer create a storage class specified by `--pv-provisioner` by default. This behavior is set by the new flag `--create-storage-class` which by default is `false`.

**Note:** If your cluster does not have the following backup policy then you can simply upgrade the operator to the `v0.6.1` image.

Backup policy that has `StorageType=PersistentVolume` but `pv.storageClass` is unset. For example:
```yaml
spec:
    backup:
      backupIntervalInSecond: 30
      maxBackups: 5
      pv:
        storageClass: ""
        volumeSizeInMB: 512
      storageType: PersistentVolume
```

So if your cluster has the above backup policy then do the following steps before upgrading the operator image to `v0.6.1`.


- Confirm the name of the storage class for a given cluster:

  ```sh
  kubectl -n <namespace> get pvc -l=etcd_cluster=<cluster-name> -o yaml | grep storage-class
  ```

- Edit your etcd cluster spec by changing the `spec.backup.pv.storageClass` field to the name of the existing storage class from the previous step.
- Wait for the the backup sidecar to be updated.

## v0.5.x -> v0.6.0

### Breaking Change

The v0.6.0 release removes [operator S3 flag](https://github.com/coreos/etcd-operator/blob/v0.5.1/doc/user/backup_config.md#operator-level-configuration).

**Note:** if your cluster is not using operator S3 flag, then you just need recreate etcd-operator deployment with the `v0.6.0` image.

If your cluster is using operator S3 flag for backup and want to use S3 backup in v0.6.0, then you need to migrate your cluster to use [cluster level backup](../backup_config.md#S3-on-aws).

Steps for migration:

- Create a backup of you current data using the [backup service guide](../backup_service.md#http-api-v1).

- Create a new AWS secret `<aws-secret>` for [cluster level backup](../backup_config.md#s3-on-aws):

  `$ kubectl -n <namespace> create secret generic <aws-secret> --from-file=$AWS_DIR/credentials --from-file=$AWS_DIR/config`

- Change the backup field in your existing cluster backup spec to enable [cluster level backup](../backup_config.md#s3-on-aws):

  From

  ```yaml
  backup:
    backupIntervalInSecond: 30
    maxBackups: 5
    storageType: "S3"
  ```

  to

  ```yaml
  backup:
    backupIntervalInSecond: 30
    maxBackups: 5
    storageType: "S3"
    s3:
      s3Bucket: <your-s3-bucket>
      awsSecret: <aws-secret> 
  ```

- Apply the cluster backup spec:
  `$ kubectl -n <namespace> apply -f <your-cluster-deployment>.yaml`

- Update deployment spec to use `etcd-operator:v0.6.0` and remove the dependency on operator level S3 backup flags:

  From

  ```yaml
  spec:
    containers:
    - name: etcd-operator
      image: quay.io/coreos/etcd-operator:v0.5.2
      command: 
        - /usr/local/bin/etcd-operator
        - --backup-aws-secret=aws
        - --backup-aws-config=aws
        - --backup-s3-bucket=<your-s3-bucket>
  ```

  to 

  ```yaml
  spec:
    containers:
    - name: etcd-operator
      image: quay.io/coreos/etcd-operator:v0.6.0
      command: 
        - /usr/local/bin/etcd-operator
  ```

- Apply the updated deployment spec:
  `$ kubectl -n <namespace> apply -f <your-etcd-operator-deployment>.yaml`

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
    - `kubectl delete thirdpartyresource cluster.etcd.coreos.com`
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
To upgrade to `v0.3.x` the current operator version **must** first be upgraded to `v0.2.6+`, since versions < `v0.2.6` are not compatible with `v0.3.x` .

### Noticeable changes:
- `Spec.Backup.MaxBackups` update:
  - If you have `Spec.Backup.MaxBackups < 0`, previously it had no effect.
    Now it will get rejected. Please remove it.
  - If you have `Spec.Backup.MaxBackups == 0`, previously it had no effect.
    Now it will create backup sidecar that allows unlimited backups.
    If you don't want backup sidecar, just don't set any backup policy.
