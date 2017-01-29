# Cluster upgrade

## etcd upgrade story

- A user “kubectl apply” a new version in EtcdCluster object
- etcd operator detects the version and
  - if the targeted version is allowed, does rolling upgrade
  - otherwise, rejects it in admission control; We will write an admission plug-in to verify EtcdCluster object.

## Diagram
![](./upgrade.jpg)

## Rolling upgrade

- We have an annotation key for the version of etcd cluster
- During upgrade, we will list pods of this cluster, and differentiate them by two versions. Then do
  - if num(old) + num(new) == total, try to update "old" pod to new version.
  - otherwise, falls to normal reconcile path.

## Support notes

- Upgrade path: We only support one minor version upgrade, e.g. 3.0 -> 3.1, no 3.0 -> 3.2. Only support 3.0+
- Rollback: We relies on etcd operator to do periodic backup.
  For alpha release, we will provide features to do manual rollback.
  In the future, we might consider support automatic rollback.


## etcd upgrade policy

- etcd supports rolling upgrade from one minor release version to its next minor release version. For example, we can directly upgrade etcd 3.0.7 to 3.1.0.
- etcd supports rolling upgrade within one minor release. For example, we can upgrade etcd 3.0.1 to 3.0.7.
- etcd supports minor version upgrade unconcerned with patch version, e.g. from 3.0.1 to 3.1.0. Nonetheless it's recommended to upgrade from 3.0.1 to 3.0.n (n is latest) and then from 3.0.n to 3.1.0.
