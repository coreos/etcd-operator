# Cluster Spec Examples

### Three members cluster

```yaml
spec:
  size: 3
```

This will use default version that etcd operator chooses.

### Three members cluster with version specified

```yaml
spec:
  size: 3
  version: "3.1.8"
```

### Three members cluster with node selector and anti-affinity

```yaml
spec:
  size: 3
  pod:
    nodeSelector:
      diskType: ssd
    antiAffinity: true
```

### Three members cluster with resource requirement

```yaml
spec:
  size: 3
  pod:
    resources:
      limits:
        cpu: 300m
        memory: 200Mi
      requests:
        cpu: 200m
        memory: 100Mi
```

### Three members cluster with PV backup

See [example](../../example/example-etcd-cluster-with-backup.yaml) .

### Three members cluster with S3 backup

```yaml
spec:
  size: 3
  backup:
    backupIntervalInSecond: 300
    maxBackups: 5
    storageType: "S3"
```

### S3 backup and cluster specific S3 configuration

```yaml
spec:
  size: 3
  backup:
    backupIntervalInSecond: 1800
    maxBackups: 5
    storageType: "S3"
    s3:
      s3Bucket: <S3-bucket-name>
      awsSecret: <aws-secret-name>
```

See [backup config](./backup_config.md) for how to set up S3 related configurations.

### Three members cluster that restores from previous PV backup

If a cluster `cluster-a` was created with backup, but deleted or failed later on,
we can recover the cluster as long as the PV still exists.
Note that delete `cluster-a` Cluster resource first if it still exists.

Here's an example:

```yaml
metadata:
  name: "cluster-a"
spec:
  size: 3
  backup:
    backupIntervalInSecond: 300
    maxBackups: 5
    storageType: "PersistentVolume"
    pv:
      volumeSizeInMB: 512
  restore:
    backupClusterName: "cluster-a"
    storageType: "PersistentVolume"
```

### Three members cluster that restores from previous S3 backup

Same as above but using "S3" as backup storage.

```yaml
metadata:
  name: "cluster-a"
spec:
  size: 3
  backup:
    backupIntervalInSecond: 300
    maxBackups: 5
    storageType: "S3"
  restore:
    backupClusterName: "cluster-a"
    storageType: "S3"
```


### Three members cluster that restores from different cluster's PV backup

If user wants to clone a new cluster `cluster-b` from an existing cluster `cluster-a`,
as long as backup exists, use the following example spec:

```yaml
metadata:
  name: "cluster-b"
spec:
  size: 3
  backup:
    backupIntervalInSecond: 300
    maxBackups: 5
    storageType: "PersistentVolume"
    pv:
      volumeSizeInMB: 512
  restore:
    backupClusterName: "cluster-a"
    storageType: "PersistentVolume"
```

## ABS spec examples

### Three member cluster with ABS backup

```yaml
spec:
  size: 3
  backup:
    backupIntervalInSecond: 1800
    maxBackups: 5
    storageType: "ABS"
    abs:
      absContainer: <abs-container-name>
      absSecret: <abs-secret-name>
  ## the section below can be added to restore from a pre-existing cluster backup
  restore:
    backupClusterName: "cluster-a"
    storageType: "ABS"
```

### TLS

See [cluster TLS docs](./cluster_tls.md).
