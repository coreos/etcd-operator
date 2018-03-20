# Installation guide

## Set up RBAC

Set up basic [RBAC rules][rbac-rules] for etcd operator:

```bash
$ example/rbac/create_role.sh
```

## Install etcd operator

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

To delete all clusters, delete all cluster CR objects before uninstalling the operator.

Clean up etcd operator:

```bash
kubectl delete -f example/deployment.yaml
kubectl delete endpoints etcd-operator
kubectl delete crd etcdclusters.etcd.database.coreos.com
kubectl delete clusterrole etcd-operator
kubectl delete clusterrolebinding etcd-operator
```

## Installation using Helm

etcd operator is available as a [Helm chart][etcd-helm]. Follow the instructions on the chart to install etcd operator on clusters.
[Alejandro Escobar][alejandroEsc] is the active maintainer.


[rbac-rules]: rbac.md
[etcd-helm]: https://github.com/kubernetes/charts/tree/master/stable/etcd-operator/
[alejandroEsc]:https://github.com/alejandroEsc