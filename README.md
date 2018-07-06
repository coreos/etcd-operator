# etcd operator
unit/integration:
[![Build Status](https://jenkins-etcd-public.prod.coreos.systems/job/etcd-operator-unit-master/badge/icon)](https://jenkins-etcd-public.prod.coreos.systems/job/etcd-operator-unit-master/lastBuild/)
e2e (Kubernetes stable):
[![Build Status](https://jenkins-etcd-public.prod.coreos.systems/buildStatus/icon?job=etcd-operator-master)](https://jenkins-etcd-public.prod.coreos.systems/job/etcd-operator-master/)
e2e (upgrade):
[![Build Status](https://jenkins-etcd.prod.coreos.systems/buildStatus/icon?job=etcd-operator-upgrade)](https://jenkins-etcd.prod.coreos.systems/job/etcd-operator-upgrade/)

### Project status: beta

Major planned features have been completed, and while no breaking API changes are currently planned, we reserve the right to address bugs and API changes in a backwards incompatible way before the project is declared stable. See [upgrade guide](./doc/user/upgrade/upgrade_guide.md) for a safe upgrade process.

Currently user facing etcd cluster objects are created as [Kubernetes Custom Resources](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/), however, taking advantage of [User Aggregated API Servers](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/api-machinery/aggregated-api-servers.md) to improve reliability, validation and versioning is planned. The use of Aggregated API should be minimally disruptive to existing users but may change what Kubernetes objects are created or how users deploy the etcd operator.

We expect to consider the etcd operator stable soon; backwards incompatible changes will not be made once the project reaches stability.

### Overview

The etcd operator manages etcd clusters deployed to [Kubernetes][k8s-home] and automates tasks related to operating an etcd cluster.

- [Create and Destroy](#create-and-destroy-an-etcd-cluster)
- [Resize](#resize-an-etcd-cluster)
- [Failover](#failover)
- [Rolling upgrade](#upgrade-an-etcd-cluster)
- [Backup and Restore](#backup-and-restore-an-etcd-cluster)

There are [more spec examples](./doc/user/spec_examples.md) on setting up clusters with different configurations

Read [Best Practices](./doc/best_practices.md) for more information on how to better use etcd operator.

Read [RBAC docs](./doc/user/rbac.md) for how to setup RBAC rules for etcd operator if RBAC is in place.

Read [Developer Guide](./doc/dev/developer_guide.md) for setting up a development environment if you want to contribute.

See the [Resources and Labels](./doc/user/resource_labels.md) doc for an overview of the resources created by the etcd-operator.

## Requirements

- Kubernetes 1.8+
- etcd 3.2.13+

## Demo

## Getting started

![etcd Operator demo](https://raw.githubusercontent.com/coreos/etcd-operator/master/doc/gif/demo.gif)

### Deploy etcd operator

See [instructions on how to install/uninstall etcd operator](doc/user/install_guide.md) .

### Create and destroy an etcd cluster

```bash
$ kubectl create -f example/example-etcd-cluster.yaml
```

A 3 member etcd cluster will be created.

```bash
$ kubectl get pods
NAME                            READY     STATUS    RESTARTS   AGE
example-etcd-cluster-gxkmr9ql7z   1/1       Running   0          1m
example-etcd-cluster-m6g62x6mwc   1/1       Running   0          1m
example-etcd-cluster-rqk62l46kw   1/1       Running   0          1m
```

See [client service](doc/user/client_service.md) for how to access etcd clusters created by the operator.

If you are working with [minikube locally](https://github.com/kubernetes/minikube#minikube), create a nodePort service and test that etcd is responding:

```bash
$ kubectl create -f example/example-etcd-cluster-nodeport-service.json
$ export ETCDCTL_API=3
$ export ETCDCTL_ENDPOINTS=$(minikube service example-etcd-cluster-client-service --url)
$ etcdctl put foo bar
```

Destroy the etcd cluster:

```bash
$ kubectl delete -f example/example-etcd-cluster.yaml
```

### Resize an etcd cluster

Create an etcd cluster:

```
$ kubectl apply -f example/example-etcd-cluster.yaml
```

In `example/example-etcd-cluster.yaml` the initial cluster size is 3.
Modify the file and change `size` from 3 to 5.

```
$ cat example/example-etcd-cluster.yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdCluster"
metadata:
  name: "example-etcd-cluster"
spec:
  size: 5
  version: "3.2.13"
```

Apply the size change to the cluster CR:
```
$ kubectl apply -f example/example-etcd-cluster.yaml
```
The etcd cluster will scale to 5 members (5 pods):
```
$ kubectl get pods
NAME                            READY     STATUS    RESTARTS   AGE
example-etcd-cluster-cl2gpqsmsw   1/1       Running   0          5m
example-etcd-cluster-cx2t6v8w78   1/1       Running   0          5m
example-etcd-cluster-gxkmr9ql7z   1/1       Running   0          7m
example-etcd-cluster-m6g62x6mwc   1/1       Running   0          7m
example-etcd-cluster-rqk62l46kw   1/1       Running   0          7m
```

Similarly we can decrease the size of the cluster from 5 back to 3 by changing the size field again and reapplying the change.

```
$ cat example/example-etcd-cluster.yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdCluster"
metadata:
  name: "example-etcd-cluster"
spec:
  size: 3
  version: "3.2.13"
```
```
$ kubectl apply -f example/example-etcd-cluster.yaml
```

We should see that etcd cluster will eventually reduce to 3 pods:

```
$ kubectl get pods
NAME                            READY     STATUS    RESTARTS   AGE
example-etcd-cluster-cl2gpqsmsw   1/1       Running   0          6m
example-etcd-cluster-gxkmr9ql7z   1/1       Running   0          8m
example-etcd-cluster-rqk62l46kw   1/1       Running   0          9mp
```

### Failover

If the minority of etcd members crash, the etcd operator will automatically recover the failure.
Let's walk through this in the following steps.

Create an etcd cluster:

```
$ kubectl create -f example/example-etcd-cluster.yaml
```

Wait until all three members are up. Simulate a member failure by deleting a pod:

```bash
$ kubectl delete pod example-etcd-cluster-cl2gpqsmsw --now
```

The etcd operator will recover the failure by creating a new pod `example-etcd-cluster-n4h66wtjrg`:

```bash
$ kubectl get pods
NAME                            READY     STATUS    RESTARTS   AGE
example-etcd-cluster-gxkmr9ql7z   1/1       Running   0          10m
example-etcd-cluster-n4h66wtjrg   1/1       Running   0          26s
example-etcd-cluster-rqk62l46kw   1/1       Running   0          10m
```

Destroy etcd cluster:
```bash
$ kubectl delete -f example/example-etcd-cluster.yaml
```

### etcd operator recovery

Let's walk through operator recovery in the following steps.

Create an etcd cluster:

```
$ kubectl create -f example/example-etcd-cluster.yaml
```

Wait until all three members are up. Then stop the etcd operator and delete one of the etcd pods:

```bash
$ kubectl delete -f example/deployment.yaml
deployment "etcd-operator" deleted

$ kubectl delete pod example-etcd-cluster-8gttjl679c --now
pod "example-etcd-cluster-8gttjl679c" deleted
```

Next restart the etcd operator. It should recover itself and the etcd clusters it manages.

```bash
$ kubectl create -f example/deployment.yaml
deployment "etcd-operator" created

$ kubectl get pods
NAME                              READY     STATUS    RESTARTS   AGE
example-etcd-cluster-m8gk76l4ns   1/1       Running   0          3m
example-etcd-cluster-q6mff85hml   1/1       Running   0          3m
example-etcd-cluster-xnfvm7lg66   1/1       Running   0          11s
```

### Upgrade an etcd cluster

Create and have the following yaml file ready:

```
$ cat upgrade-example.yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdCluster"
metadata:
  name: "example-etcd-cluster"
spec:
  size: 3
  version: "3.1.10"
  repository: "quay.io/coreos/etcd"
```

Create an etcd cluster with the version specified (3.1.10) in the yaml file:

```
$ kubectl apply -f upgrade-example.yaml
$ kubectl get pods
NAME                              READY     STATUS    RESTARTS   AGE
example-etcd-cluster-795649v9kq   1/1       Running   1          3m
example-etcd-cluster-jtp447ggnq   1/1       Running   1          4m
example-etcd-cluster-psw7sf2hhr   1/1       Running   1          4m
```

The container image version should be 3.1.10:

```
$ kubectl get pod example-etcd-cluster-795649v9kq -o yaml | grep "image:" | uniq
    image: quay.io/coreos/etcd:v3.1.10
```

Now modify the file `upgrade-example` and change the `version` from 3.1.10 to 3.2.13:

```
$ cat upgrade-example
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdCluster"
metadata:
  name: "example-etcd-cluster"
spec:
  size: 3
  version: "3.2.13"
```

Apply the version change to the cluster CR:

```
$ kubectl apply -f upgrade-example
```

Wait ~30 seconds. The container image version should be updated to v3.2.13:

```
$ kubectl get pod example-etcd-cluster-795649v9kq -o yaml | grep "image:" | uniq
    image: gcr.io/etcd-development/etcd:v3.2.13
```

Check the other two pods and you should see the same result.


### Backup and Restore an etcd cluster
> Note: The provided etcd backup/restore operators are example implementations.

Follow the [etcd backup operator walkthrough](./doc/user/walkthrough/backup-operator.md) to backup an etcd cluster.

Follow the [etcd restore operator walkthrough](./doc/user/walkthrough/restore-operator.md) to restore an etcd cluster on Kubernetes from backup.

### Manage etcd clusters in all namespaces

See [instructions on clusterwide feature](doc/user/clusterwide.md).

[k8s-home]: http://kubernetes.io
