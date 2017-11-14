# Operation Guide

## Install etcd operator

Set up basic [RBAC rules](./rbac.md) for etcd operator:

```bash
$ example/rbac/create_role.sh
```

Create a deployment for etcd operator:

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

Cleanup etcd operator:

```bash
kubectl delete -f example/deployment.yaml
kubectl delete endpoints etcd-operator
kubectl delete clusterrole etcd-operator
kubectl delete clusterrolebinding etcd-operator
```

## Installation via Helm
**Disclaimer:** The following Helm chart is an external project not maintained by the etcd-operator maintainers; so it may not be up to date.

etcd-operator is available as a [Helm
chart](https://github.com/kubernetes/charts/tree/master/stable/etcd-operator).
Follow the instructions on the chart to install etcd-operator on your cluster.
 
