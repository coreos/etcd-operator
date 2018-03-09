## Cluster status reporting

An etcd cluster permanently fails when all its members are dead and no backup is available. A user might update etcd cluster TPR with invalid input or format. The operator needs to notify users about these bad events.

A common way to do this in the Kubernetes world is through status field, like pod.status. The etcd operator will write out cluster status similar to pod status.

```go
type EtcdCluster struct {
    unversioned.TypeMeta `json:",inline"`
    api.ObjectMeta       `json:"metadata,omitempty"`
    Spec                 ClusterSpec `json:"spec"`
    Status               ClusterStatus `json:"status"`
}

Type ClusterStatus struct {
    Paused bool `json:"paused"`
    ...
}
```

The etcd operator keeps the truth of the cluster status, and update the TPR item after each cluster reconciliation atomically. Note that users can:

 - modify TPR item concurrently with the operator.
 - update TPR and overwrite the status filed with empty or bad input

 The operator MUST handle these two cases gracefully. Thus, the operator MUST NOT overwrite the user input on spec field. The operator CANNOT trust the existing status filed in TPR.

To not overwrite the user input, the etcd operator will set resource version, and do a compare and swap on resource version to update the status. If the compare and swap fails, the operator knows that the user updated the spec concurrently, and it will retry later after get the new spec.

To not be affected by the potential empty or broken status accidentally written by the user, the etcd operator will never read the status from TPR. It always keeps the source of truth in memory.

In summary, the TPR:

- receive spec update (or initialize spec) with a resource version
- collect cluster status during reconciliation
- atomically update cluster status with known resource version after reconciliation if there is a status change
  - retry if resource version does not match
