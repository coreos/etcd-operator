# Custom Resource Migration Guide for 0.7.0 relase

In 0.7.0 release, BackupPolicy and RestorePolicy are removed from EtcdCluster spec.
This doc provides the migration guide for the change.

## Migrate EtcdCluster

This has to be done **before upgrading operator**.

Update any EtcdCluster Custom Resource to remove BackupPolicy and RestorePolicy:

```
kubectl get etcdcluster <cluster-name> -o json | \
	jq 'del(.spec.backup)' | jq 'del(.spec.restore)' | \
	kubectl replace -f -
```

## Migrate BackupPolicy to EtcdBackup CR

Read [etcd backup operator doc](../walkthrough/backup-operator.md) for how to save backup for etcd cluster.
It currently only supports S3 backup for non-TLS etcd cluster.

For previous backup policy, for example:

```yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdCluster"
metadata:
  name: "example-etcd-cluster"
spec:
  ...
  backup:
    storageType: "S3"
    s3:
      s3Bucket: <s3-bucket-name>
      awsSecret: <aws-secret-name>
```

The equivalent EtcdBackup CR is:

```yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdBackup"
metadata:
  name: example-etcd-cluster-backup
spec:
  clusterName: example-etcd-cluster
  storageType: "S3"
  s3:
    s3Bucket: <s3-bucket-name>
    awsSecret: <aws-secret-name>
```

Create the above CR will make **one-time** backup attempt to target etcd cluster.
After backup is saved, it is safe to delete the CR and doing that won’t delete the S3 backup.


## Migrate RestoreSpec to EtcdRestore CR

Read [etcd restore operator doc](../walkthrough/restore-operator.md) for how to restore etcd cluster from backup.
It currently only supports S3 backup and non-TLS etcd cluster.

For previous restore policy for example:
```yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdCluster"
metadata:
  name: "restored-etcd-cluster"
spec:
  ...
  restore:
    storageType: "S3"
    backupClustername: “example-etcd-cluster”
```

The equivalent EtcdRestore CR is:

```yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdRestore"
metadata:
  # an EtcdCluster with the same name will be created
  name: "restored-etcd-cluster"
spec:
  clusterSpec:
    size: 3
    version: "3.1.8"
  s3:
    # e.g: "etcd-snapshot-bucket/v1/default/example-etcd-cluster/3.1.8_0000000000000001_etcd.backup"
    path: <full-s3-path>
    awsSecret: <aws-secret>
```

Create the above CR to trigger restore.

> Note: restore process will create a new EtcdCluster CR.

Wait until etcd cluster reaches given size.
The it is safe to delete the EtcdRestore CR and doing that won’t delete the EtcdCluster CR.
