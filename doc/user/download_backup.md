# Download Backup

The backups that etcd operator saves are [etcd snapshots](https://github.com/coreos/etcd/blob/master/Documentation/op-guide/recovery.md).
This document talks about the ways to download these backup/snapshot files.

## Backup Service

If etcd cluster with backup is still running, there is a backup service.
It's named as `${CLUSTER_NAME}-backup-sidecar`. For example:
```
$ kubectl get svc
etcd-cluster-backup-sidecar   10.39.243.229   <none>        19999/TCP           4m
```

Given the etcd version of the cluster, you can download backup via service endpoint
`http://${CLUSTER_NAME}-backup-sidecar:19999/v1/backup?etcdVersion=${ETCD_VERSION}` . For example:
```
$ curl "http://etcd-cluster-backup-sidecar:19999/v1/backup?etcdVersion=3.1.0" -o 3.1.0_etcd.backup
```
"etcdVersion" parameter is the version of the potential etcd cluster to restore and run.
On success, the backup's etcd version should be compatible with given version.
Otherwise, it would return non-OK response.

If sending the request from a pod in a different namespace, use FQDN `${CLUSTER_NAME}-backup-sidecar.${NAMESPACE}.svc.cluster.local` .

Outside kubernetes cluster, we need additional step to access the backup service,
e.g. using [ingress](https://kubernetes.io/docs/user-guide/ingress/) .

## Get backup from S3

If backup service is healthy, we suggest to get backups via that.

However, in disaster scenarios like when Kubernetes is down, the backup service is not running, 
and we cannot retrieve backup from backup service.

If backup storage type is "S3", users have to get the backup directly from S3.

First of all, setup aws cli: https://aws.amazon.com/cli/ .

Given the S3 bucket name that you passed to when starting etcd operator and the cluster name,
backups are saved under prefix `${BUCKET_NAME}/v1/${CLUSTER_NAME}/` .

List all backup files:
```
$ aws s3 ls ${BUCKET_NAME}/v1/${CLUSTER_NAME}/
2017-01-24 02:13:30    24608    3.1.0_0000000000000002_etcd.backup
...                             3.1.0_000000000000000f_etcd.backup
```

Backup file name format is `${ETCD_VERSION}_${CLUSTER_REVISION}_etcd.backup` . Revision is hexadecimal.

Unless intentional, just pick the backup with max revision. 
E.g. in above examples, we would pick file "3.1.0_000000000000000f_etcd.backup" .

Download backup:
```
$ aws s3 cp "s3://${BUCKET_NAME}/v1/${CLUSTER_NAME}/${MAX_REVISION_BACKUP}" $target_local_file
```

## Get backup from PV
If backup service is healthy, we suggest to get backups via that.

However, in disaster scenarios like when Kubernetes is down, the backup service is not running,
and we cannot retrieve backup from backup service.

If backup storage type is "PV", users need to get backup directly from PV.

TODO: document how to find and mount the disk.

Given the cluster name, backups are stored under dir `/var/etcd-backup/v1/${CLUSTER_NAME}/` .

List all backup files:
```
$ ls /var/etcd-backup/v1/${CLUSTER_NAME}/
3.1.0_0000000000000002_etcd.backup 3.1.0_000000000000000f_etcd.backup ...
```

Backup file name format is `${ETCD_VERSION}_${CLUSTER_REVISION}_etcd.backup` . Revision is hexadecimal.

Unless intentional, just pick the backup with max revision.
E.g. in above examples, we would pick file "3.1.0_000000000000000f_etcd.backup" .

TODO: document how to download.
