## Resource Labels
The etcd-operator creates the following Kubernetes resources for each etcd cluster:
- Pods for the etcd nodes
- Deployment for the backup side car
- Services for the etcd client, peer and backup service
- Persistent Volume Claim, if backup is enabled with the storage type as PersistentVolume

where each resource has the following labels:
- `app=etcd`
- `etcd_cluster=<cluster-name>`
