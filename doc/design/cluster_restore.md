# Restore Cluster From Existing Backup

We want to restore an etcd cluster from existing backup that operator has saved before.

We add a field in ClusterSpec:

```Go
type ClusterSpec struct {
    ...
    // RestorePolicy defines the policy to restore cluster form existing backup if not nil.
    RestorePolicy *RestorePolicy
}

type RestorePolicy struct {
    // BackupClusterName is the cluster name of the backup to recover from.
    BackupClusterName string

    // StorageType specifies the type of storage device to store backup files.
    // If it's not set by user, the default is "PersistentVolume".
    StorageType BackupStorageType
}
```

For example, let's say, a user has a cluster "etcd-A" running. For some reason, it stops working.
At the end, the cluster is killed by user or dead by itself. Fortunately, the cluster was run with
valid BackupPolicy and saved with backup. Now, user can restore the cluster by adding "restorePolicy"
to previous yaml file:

```yaml
apiVersion: "etcd.coreos.com/v1beta1"
kind: "Cluster"
metadata:
  name: "etcd-A"
spec:
  ...
  restorePolicy:
    backupClusterName: "etcd-A"
    storageType: "PersistentVolume"
```

It is not necessary to restore the exact same cluster as before.
User can restore a new cluster "etcd-B" from "etcd-A", with different size, backup policy, etc.
