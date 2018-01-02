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
  version: "3.2.13"
```

## Three member cluster with node selector and anti-affinity across nodes

> Note: change $cluster_name to the EtcdCluster's name.

```yaml
spec:
  size: 3
  pod:
    nodeSelector:
      diskType: ssd
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
            - key: etcd_cluster
              operator: In
              values: ["$cluster_name"]
          topologyKey: kubernetes.io/hostname
```

For other topology keys, see https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ .

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


[cluster-tls]: cluster_tls.md
