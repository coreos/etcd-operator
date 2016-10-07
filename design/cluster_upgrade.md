# Cluster upgrade

- etcd supports rolling upgrade from one minor release version to its next minor release version. For example, we can directly upgrade etcd 3.0.7 to 3.1.0.
- etcd supports rolling upgrade within one minor release. For example, we can upgrade etcd 3.0.1 to 3.0.7.
- etcd supports minor version upgrade unconcerned with patch version, e.g. from 3.0.1 to 3.1.0. Nonetheless it's recommended to upgrade from 3.0.1 to 3.0.n (n is latest) and then from 3.0.n to 3.1.0.

## Rolling upgrade

We upgrade one member at a time during a cluster upgrade. To upgrade that member, we simply update the pod manifest with the desired version of etcd container. We need to clear all --initial prefix flags previously set.

