# Incompatible Upgrade Guide
For the case when the desired operator version is not compatible with an existing cluster created by a previous operator version, the cluster will need to be recreated from a backup under the new operator. 

This is a more general way to upgrade the operator when an in-place update is not possible.


First make a backup of the current cluster. See the [backup service](../backup_service.md) doc to see how to do this from inside the kubernetes cluster.

Next delete the current cluster and operator. Make sure that the `cleanupBackupsOnClusterDelete` field was not set to `true` for the cluster configuration since that would clean the backup upon deleting the cluster.
```bash
$ kubectl delete cluster <cluster-name>
$ kubectl delete deployment etcd-operator
```

Now modify the yaml/json spec file used to create the original operator deployment by setting the value of the `spec.template.spec.containers.image` field to the desired image version like `quay.io/coreos/etcd-operator:v0.2.5` .

Then simply create operator using the updated deployment spec:
```bash
$ kubectl create -f deployment.yaml
```

To restore from the backup, the new cluster spec will look similar to the template below depending on your type of storage used for the backup:
```YAML
apiVersion: "etcd.coreos.com/v1beta1"
kind: "Cluster"
metadata:
  name: <new-cluster-name> # can be the same as the old cluster name
spec:
  size: <cluster-size>
  version: <etcd-cluster-version>
  backup:
    backupIntervalInSecond: <interval>
    maxBackups: <num-backups>
    storageType: <type> # S3 or PV
  restore:
    backupClusterName: <previous-cluster-name>
    storageType: <type> # S3 or PV
```

Finally restore the new cluster from the backup of the previous cluster:
```bash
$ kubectl create -f cluster-restore-from-backup.yaml
```
