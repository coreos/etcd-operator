# Cluster Spec Examples

## Three member cluster

```yaml
spec:
  size: 3
```

This will use the default version chosen by the etcd-operator.

## Three member cluster with version specified

```yaml
spec:
  size: 3
  version: "3.2.10"
```

## Three member cluster with node selector and anti-affinity

```yaml
spec:
  size: 3
  pod:
    nodeSelector:
      diskType: ssd
    antiAffinity: true
```

## Three member cluster with resource requirement

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
## TLS

For more information on working with TLS, see [Cluster TLS policy][cluster-tls].


[cluster-tls]: cluster_tls.html
