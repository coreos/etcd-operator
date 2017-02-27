# Writing Cluster Spec
 
Users tell operator the desired state of an etcd cluster via spec in `Cluster` third party resource (TPR).
See [example](../../example/example-etcd-cluster.yaml).

Users might wonder what to specify in the spec.
Spec is equivalent to [ClusterSpec](https://github.com/coreos/etcd-operator/blob/v0.2.1/pkg/spec/cluster.go#L67).
When you submit yaml/json to APIServer, it will be converted to `ClusterSpec` go struct.

You can find docs on [godoc](https://godoc.org/github.com/coreos/etcd-operator/pkg/spec#ClusterSpec).
If we don't mention any default value, it will use default value in go.

TODO: We would provide no go knowledge needed, versioned docs in the future.

## Examples

Create a three-members cluser with [NodeSelector](https://kubernetes.io/docs/user-guide/node-selection/),
anti-affinity constraint across pods of same cluster, resource requirement:
```yaml
spec:
  size: 3
  pod:
    nodeSelector:
      diskType: ssd
    antiAffinity: true
    resourceRequirements:
      limits:
        cpu: 300m
        memory: 200Mi
      requests:
        cpu: 200m
        memory: 100Mi
```

There is an [example cluster with PV backup](../../example/example-etcd-cluster-with-backup.yaml).
Here is a similar example that stores backup in "S3":
```yaml
spec:
  size: 3
  version: "3.1.0"
  backup:
    backupIntervalInSecond: 300
    maxBackups: 5
    storageType: "S3"
```
