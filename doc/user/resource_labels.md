## Resource Labels
The etcd-operator creates the following Kubernetes resources for each etcd cluster:
- Pods for the etcd nodes
- Services for the etcd client, peer, and (optional) backup service
- (Optional) Deployment for the backup sidecar, if backup spec is defined
- (Optional) Persistent Volume Claim, if backup sidecar is enabled with the storage type as PersistentVolume

where each resource has the following labels:
- `app=etcd`
- `etcd_cluster=<cluster-name>`
