# Operation Guide

## Install etcd operator

Before you create the etcd-operator make sure you setup the necessary [rbac rules](./rbac.md) if your Kubernetes version is 1.7 or higher.

Create deployment:

```bash
$ kubectl create -f example/deployment.yaml
```

etcd operator will automatically create a Kubernetes Custom Resource Definition (CRD):

```bash
$ kubectl get customresourcedefinitions
NAME                                    KIND
etcdclusters.etcd.database.coreos.com   CustomResourceDefinition.v1beta1.apiextensions.k8s.io
```

## Uninstall etcd operator

Note that the etcd clusters managed by etcd operator will **NOT** be deleted even if the operator is uninstalled.
This is an intentional design to prevent accidental operator failure from killing all the etcd clusters.
In order to delete all clusters, delete all cluster CR objects before uninstall the operator.

Delete deployment:

```bash
$ kubectl delete -f example/deployment.yaml
```

Delete leader election endpoint
```bash
$ kubectl delete endpoints etcd-operator
```

## Installation via Helm

etcd-operator is available as a [Helm
chart](https://github.com/kubernetes/charts/tree/master/stable/etcd-operator).
Follow the instructions on the chart to install etcd-operator on your cluster.
 
