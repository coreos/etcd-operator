# Manage clusters in all namespaces

Default etcd operator behavior is to only manage etcd clusters created in the same namespace.
It is possible to deploy an etcd operator with special option to manage clusterwide etcd clusters.

## Install etcd operator

etcd operator have to run with `-cluster-wide` arg option.

More information in [install guide](doc/user/install_guide.md).

## Special annotation

To declare an etcd cluster as "clusterwide", you have to add special annotation `etcd.database.coreos.com/scope` with value `clusterwide`.

```yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdCluster"
metadata:
  name: "example-etcd-cluster"
  annotations:
    etcd.database.coreos.com/scope: clusterwide
spec:
  size: 3
  version: "3.2.13"
```
