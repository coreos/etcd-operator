# Restore Operator Design

Restore operator restores an etcd cluster from existing backup.

## General design

The new design will assume EtcdCluster have three phases:

- **Creating**: etcd operator will create a seed member.
- **Running**: etcd operator will keep reconciling cluster membership, 
  actual running pods, and desired size.
- **Failed**: Encountered unrecoverable failure, e.g. lose majority of the cluster.

For this design, restore operator is going to restore an etcd seed member that
etcd operator understands. etcd operator will skip Creating phase and jump directly
into Running phase.

## Restore operator API

Restore operator API will be exposed as CRD:

```yaml
apiVersion: apiextentions.k8s.io/v1beta2
kind: CustomResourceDefiniton
metadata:
  name: etcdrestores.etcd.database.coreos.com
spec:
  group: etcd.database.coreos.com
  version: v1beta2
  Scope: Namespaced
  names: 
    kind: EtcdRestore
    plural: etcdrestores
```

Restore Spec defined as:

```Go
// RestoreSpec defines how to restore an etcd cluster from existing backup.
type RestoreSpec struct {
	// EtcdCluster defines the same spec that etcd operator will run later.
	// Using this spec, restore operator will prepare the seed that
	// etcd operator will pick up later.
	EtcdCluster ClusterSpec
	// EtcdBackup defines the same spec that backup operator uses to save the backup.
	// Restore operator will have the same logic as backup operator to discover
	// any existing backups and find the one with largest revision.
	EtcdBackup EtcdBackupSpec
}


// RestoreStatus reports the status of this restore operation.
type RestoreStatus struct {
	// Succeeded indicates if the restore has Succeeded.
	Succeeded bool
	// Reason indicates the reason for any restore related failures.
	Reason string
}
```
