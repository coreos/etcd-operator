# Manage clusters in all namespaces

Default etcd operator behavior is to only manage etcd clusters created in the same namespace.
It is possible to deploy an etcd operator with special option to manage clusterwide etcd clusters.

## Install etcd operator

etcd operator have to run with `-cluster-wide` arg option.

More information in [install guide](install_guide.md).

See the example in [example/example-etcd-cluster.yaml](../../example/example-etcd-cluster.yaml)
