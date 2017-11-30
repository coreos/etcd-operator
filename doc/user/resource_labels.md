# Resource Labels

The etcd operator creates the following Kubernetes resources for each etcd cluster:
- Pods for the etcd nodes
- Services for the etcd client and peer

where each resource has the following labels:
- `app=etcd`
- `etcd_cluster=<cluster-name>`
