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
### TLS

See [cluster TLS docs](./cluster_tls.md).
