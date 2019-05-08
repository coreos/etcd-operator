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

## Three member cluster with preferred anti-affinity between pods and nodes (place pods in different nodes if possible)

> Note: change $cluster_name to the EtcdCluster's name.

```yaml
spec:
  size: 3
  pod:
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchExpressions:
              - key: etcd_cluster
                operator: In
                values:
                - $cluster_name
            topologyKey: kubernetes.io/hostname
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

## Custom etcd configuration

etcd members could be configured via env: https://coreos.com/etcd/docs/latest/op-guide/configuration.html

```yaml
spec:
  size: 3
  pod:
    etcdEnv:
    - name: ETCD_AUTO_COMPACTION_RETENTION
      value: "1"
```

## TLS

For more information on working with TLS, see [Cluster TLS policy][cluster-tls].

## Custom pod annotations

```yaml
spec:
  size: 3
  pod:
    annotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "2379"
```

## Custom pod security context

For more information on pod security context see the Kubernetes [docs][pod-security-context].

```yaml
spec:
  size: 3
  pod:
    securityContext:
      runAsNonRoot: true
      runAsUser: 9000
      # The FSGroup is needed to let the etcd container access mounted volumes
      fsGroup: 9000
```

## Custom PersistentVolumeClaim definition

> Note: Change $STORAGECLASS for your preferred StorageClass or remove the line to use the default one. 

```yaml
spec:
  size: 3
  pod:
    persistentVolumeClaimSpec:
      storageClassName: $STORAGECLASS
      accessModes:
      - ReadWriteOnce
      resources:
        requests:
          storage: 1Gi
```

[cluster-tls]: cluster_tls.md
[pod-security-context]: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/#set-the-security-context-for-a-pod
