# Operation Guide

## Install etcd operator

Before you create the etcd-operator make sure you setup the necessary [rbac rules](./rbac.md) if your Kubernetes version is 1.6 or higher.

Create deployment:

```bash
$ kubectl create -f example/deployment.yaml
```

etcd operator will automatically creates a Kubernetes Third-Party Resource (TPR):

```bash
$ kubectl get thirdpartyresources
NAME                      DESCRIPTION             VERSION(S)
cluster.etcd.coreos.com   Managed etcd clusters   v1beta1
```

## Uninstall etcd operator

Note that the etcd clusters managed by etcd operator will **NOT** be deleted even if the operator is uninstalled.
This is an intentional design to prevent accidental operator failure from killing all the etcd clusters.
In order to delete all clusters, delete all cluster TPR objects before uninstall the operator.

Delete deployment:

```bash
$ kubectl delete -f example/deployment.yaml
```

Delete leader election endpoint
```bash
$ kubectl delete endpoints etcd-operator
```
