# Cluster upgrade

etcd supports rolling upgrade from one minor release version to its next minor release version. For example, we can directly upgrade etcd 2.3.1 to 2.3.2. But it is usually unsupported to upgrade etcd from one version to its next next release version. For example, we should not directly upgrade etcd 2.3.1 to 2.3.3. So we should update etcd one version a time.

etcd supports rolling upgrade within one minor release. For example, we can directly upgrade a 2.3.1 cluster directly to 2.3.8.

## Rolling upgrade

We upgrade one member a time duing a cluster upgrade. To upgrade that member, we simply update the pod manifest with the desired version of etcd container. We need to clear all --initial prefix flags previously set.



