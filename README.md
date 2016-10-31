# kube-etcd-controller

[![Build Status](https://jenkins-etcd-public.prod.coreos.systems/buildStatus/icon?job=etcd-controller-master)](https://jenkins-etcd-public.prod.coreos.systems/job/etcd-controller-master/)

**Project status: *alpha***

The `etcd operator` project now ships with a lot of execiting features. We strongly encourage you to give it a try, and provide early feedbacks. It would be a great help for us to keep on improving and sharping this project. However, please do notice that this project is still in alpha stage. Not all planned feature are completed. The API, spec, status and other user facing objects are subject to change. We do not support backward-compability for the alpha releases.

## Overview

Kube-etcd-controller manages etcd clusters atop [Kubernetes][k8s-home], automating their creation and administration:

- [Create](#create-an-etcd-cluster)
- [Destroy](#destroy-an-existing-etcd-cluster)
- [Resize](#resize-an-etcd-cluster)
- [Recover a member](#member-recovery)
- [Backup and restore a cluster](#disaster-recovery)
- Cluster migration (TODO)
 - Migrate an existing etcd cluster into controller management
- [rolling upgrade](#try-upgrade-etcd-cluster)

## Requirements

- Kubernetes 1.4+
- etcd 3.0+

## Limitations

- Backup works only for data in etcd3 storage, not for data in etcd2 storage.
- Migration, the process of allowing the controller to manage existing etcd3 clusters, only supports a single-member cluster, with all nodes running in the same Kubernetes cluster.

## Deploy kube-etcd-controller

```bash
$ kubectl create -f example/etcd-controller.yaml
pod "kube-etcd-controller" created
```

kube-etcd-controller will create a Kubernetes *Third-Party Resource* (TPR) called "etcd-cluster", and an "etcd-controller-backup" storage class.

```bash
$ kubectl get thirdpartyresources
NAME                      DESCRIPTION             VERSION(S)
etcd-cluster.coreos.com   Managed etcd clusters   v1
```

## Create an etcd cluster

```bash
$ kubectl create -f example/example-etcd-cluster.yaml
```

```bash
$ kubectl get pods
NAME                             READY     STATUS    RESTARTS   AGE
etcd-cluster-0000                1/1       Running   0          23s
etcd-cluster-0001                1/1       Running   0          16s
etcd-cluster-0002                1/1       Running   0          8s
etcd-cluster-backup-tool-rhygq   1/1       Running   0          18s
```

```bash
$ kubectl get services
NAME                       CLUSTER-IP     EXTERNAL-IP   PORT(S)             AGE
etcd-cluster-0000          10.0.84.34     <none>        2380/TCP,2379/TCP   37s
etcd-cluster-0001          10.0.51.78     <none>        2380/TCP,2379/TCP   30s
etcd-cluster-0002          10.0.140.141   <none>        2380/TCP,2379/TCP   22s
etcd-cluster-backup-tool   10.0.59.243    <none>        19999/TCP           32s
```

```bash
$ kubectl logs etcd-cluster-0000
...
2016-08-05 00:33:32.453768 I | api: enabled capabilities for version 3.0
2016-08-05 00:33:32.454178 N | etcdmain: serving insecure client requests on 0.0.0.0:2379, this is strongly discouraged!
```

If you are working with [minikube locally](https://github.com/kubernetes/minikube#minikube) create a nodePort service and test out that etcd is responding:

```bash
kubectl create -f example/example-etcd-cluster-nodeport-service.json
export ETCDCTL_API=3
export ETCDCTL_ENDPOINTS=$(minikube service etcd-cluster-client-service --url)
etcdctl put foo bar
```

## Resize an etcd cluster

`kubectl apply` doesn't work for TPR at the moment. See [kubernetes/#29542](https://github.com/kubernetes/kubernetes/issues/29542). As a workaround, we use cURL to resize the cluster.

Use kubectl to create a reverse proxy:

```
$ kubectl proxy --port=8080
Starting to serve on 127.0.0.1:8080
```
Now we can talk to apiserver via "http://127.0.0.1:8080".

Create a json file with the new configuration:

```
$ cat body.json
{
  "apiVersion": "coreos.com/v1",
  "kind": "EtcdCluster",
  "metadata": {
    "name": "etcd-cluster",
    "namespace": "default"
  },
  "spec": {
    "backup": {
      "maxSnapshot": 5,
      "snapshotIntervalInSecond": 30,
      "volumeSizeInMB": 512
    },
    "size": 5
  }
}
```

In another terminal, use the following command to change the cluster size from 3 to 5.

```
$ curl -H 'Content-Type: application/json' -X PUT --data @body.json http://127.0.0.1:8080/apis/coreos.com/v1/namespaces/default/etcdclusters/etcd-cluster

{  
   "apiVersion":"coreos.com/v1",
   "kind":"EtcdCluster",
   "metadata":{  
      "name":"etcd-cluster",
      "namespace":"default",
      "selfLink":"/apis/coreos.com/v1/namespaces/default/etcdclusters/etcd-cluster",
      "uid":"4773679d-86cf-11e6-9086-42010af00002",
      "resourceVersion":"438492",
      "creationTimestamp":"2016-09-30T05:32:29Z"
   },
   "spec":{  
      "backup":{  
         "maxSnapshot":5,
         "snapshotIntervalInSecond":30,
         "volumeSizeInMB":512
      },
      "size":5
   }
}
```

We should see

```
$ kubectl get pods
NAME                             READY     STATUS              RESTARTS   AGE
etcd-cluster-0000                1/1       Running             0          3m
etcd-cluster-0001                1/1       Running             0          2m
etcd-cluster-0002                1/1       Running             0          2m
etcd-cluster-0003                1/1       Running             0          9s
etcd-cluster-0004                0/1       ContainerCreating   0          1s
etcd-cluster-backup-tool-e9gkv   1/1       Running             0          2m
```

Now we can decrease the size of cluster from 5 back to 3.

Create another json file with the cluster size specified back to 3:

```
$ cat body.json
{
  "apiVersion": "coreos.com/v1",
  "kind": "EtcdCluster",
  "metadata": {
    "name": "etcd-cluster",
    "namespace": "default"
  },
  "spec": {
    "backup": {
      "maxSnapshot": 5,
      "snapshotIntervalInSecond": 30,
      "volumeSizeInMB": 512
    },
    "size": 3
  }
}
```

Send it to API Server:

```
$ curl -H 'Content-Type: application/json' -X PUT --data @body.json http://127.0.0.1:8080/apis/coreos.com/v1/namespaces/default/etcdclusters/etcd-cluster
```

We should see that etcd cluster eventually reduces to 3 pods:

```
$ kubectl get pods
NAME                             READY     STATUS    RESTARTS   AGE
etcd-cluster-0002                1/1       Running   0          3m
etcd-cluster-0003                1/1       Running   0          1m
etcd-cluster-0004                1/1       Running   0          1m
etcd-cluster-backup-tool-e9gkv   1/1       Running   0          3m
```

## Destroy an existing etcd cluster

```bash
$ kubectl delete -f example/example-etcd-cluster.yaml
```

```bash
$ kubectl get pods
NAME                       READY     STATUS    RESTARTS   AGE
```

## Member recovery

If the minority of etcd members crash, the etcd controller will automatically recover the failure.
Let's walk through in the following steps.

Redo the "create" process to have a cluster with 3 members.

Simulate a member failure by deleting a pod:

```bash
$ kubectl delete pod etcd-cluster-0000
```

The etcd controller will recover the failure by creating a new pod `etcd-cluster-0003`

```bash
$ kubectl get pods
NAME                       READY     STATUS    RESTARTS   AGE
etcd-cluster-0001   1/1       Running   0          5s
etcd-cluster-0002   1/1       Running   0          5s
etcd-cluster-0003   1/1       Running   0          5s
```

### Controller recovery

If the etcd controller restarts, it can recover its previous state.

Continued from above, you can simulate a controller crash and a member crash:

```bash
$ kubectl delete -f example/etcd-controller.yaml
pod "kube-etcd-controller" deleted

$ kubectl delete etcd-cluster-0001
pod "etcd-cluster-0001" deleted
```

Then restart the etcd controller. It should automatically recover itself. It also recovers the etcd cluster:

```bash
$ kubectl create -f example/etcd-cluster.yaml
pod "kube-etcd-controller" created
$ kubectl get pods
NAME                READY     STATUS    RESTARTS   AGE
etcd-cluster-0002   1/1       Running   0          4m
etcd-cluster-0003   1/1       Running   0          4m
etcd-cluster-0004   1/1       Running   0          6s
```

## Disaster recovery

If the majority of etcd members crash, but at least one backup exists for the cluster, the etcd controller can restore the entire cluster from the backup.

By default, the etcd controller creates a storage class on initialization:

```
$ kubectl get storageclass
NAME                     TYPE
etcd-controller-backup   kubernetes.io/gce-pd
```

This is used to request the persistent volume to store the backup data. (TODO: We are planning to support AWS EBS soon.)

Continued from last example, a persistent volume is claimed for the backup pod:

```
$ kubectl get pvc
NAME               STATUS    VOLUME                                     CAPACITY   ACCESSMODES   AGE
pvc-etcd-cluster   Bound     pvc-164d18fe-8797-11e6-a8b4-42010af00002   1Gi        RWO           14m
```

Let's try to write some data into etcd:

```
$ kubectl run --rm -i --tty fun --image quay.io/coreos/etcd --restart=Never -- /bin/sh
/ # ETCDCTL_API=3 etcdctl --endpoints http://etcd-cluster-0002:2379 put foo bar
OK
(ctrl-D to exit)
```

Now let's kill two pods to simulate a disaster failure:

```
$ kubectl delete pod etcd-cluster-000 etcd-cluster-0003
pod "etcd-cluster-0002" deleted
pod "etcd-cluster-0003" deleted
```

Now quorum is lost. The etcd controller will start to recover the cluster by:
- Creating a new seed member to recover from the backup
- Add the specified number of members into the seed cluster

```
$ kubectl get pods
NAME                             READY     STATUS     RESTARTS   AGE
etcd-cluster-0005                0/1       Init:0/2   0          11s
etcd-cluster-backup-tool-e9gkv   1/1       Running    0          18m
...
$ kubectl get pods
NAME                             READY     STATUS    RESTARTS   AGE
etcd-cluster-0005                1/1       Running   0          3m
etcd-cluster-0006                1/1       Running   0          3m
etcd-cluster-0007                1/1       Running   0          3m
etcd-cluster-backup-tool-e9gkv   1/1       Running   0          22m
```

Note: Sometimes member recovery can fail because of a race caused by a delay in pod deletion. The process will be retried in this event.

## Upgrade an etcd cluster

Clean up any existing etcd cluster, but keep the etcd controller running.

Have the following yaml file ready:

```
$ cat 3.0-etcd-cluster.yaml
apiVersion: "coreos.com/v1"
kind: "EtcdCluster"
metadata:
  name: "etcd-cluster"
spec:
  size: 3
  version: "v3.0.12"
```

Create an etcd cluster with the version specified (3.0.12) in the yaml file:

```
$ kubectl create -f 3.0-etcd-cluster.yaml
$ kubectl get pods
NAME                   READY     STATUS    RESTARTS   AGE
etcd-cluster-0000      1/1       Running   0          37s
etcd-cluster-0001      1/1       Running   0          25s
etcd-cluster-0002      1/1       Running   0          14s
```

The container image version should be 3.0.12:

```
$ kubectl get pod etcd-cluster-0000 -o yaml | grep "image:" | uniq
    image: quay.io/coreos/etcd:v3.0.12
```

`kubectl apply` doesn't work for TPR at the moment. See [kubernetes/#29542](https://github.com/kubernetes/kubernetes/issues/29542).
We use cURL to update the cluster as a workaround.

Use kubectl to create a reverse proxy:

```
$ kubectl proxy --port=8080
Starting to serve on 127.0.0.1:8080
```

Have following json file ready:
(Note that the version field is changed from v3.0.12 to v3.1.0-alpha.1)

```
$ cat body.json
{
  "apiVersion": "coreos.com/v1",
  "kind": "EtcdCluster",
  "metadata": {
    "name": "etcd-cluster"
  },
  "spec": {
    "size": 3,
    "version": "v3.1.0-alpha.1"
  }
}
```

Then we update the version in spec.

```
$ curl -H 'Content-Type: application/json' -X PUT --data @body.json \
    http://127.0.0.1:8080/apis/coreos.com/v1/namespaces/default/etcdclusters/etcd-cluster
```

Wait ~30 seconds. The container image version should be updated to v3.1.0-alpha.1:

```
$ kubectl get pod etcd-cluster-0000 -o yaml | grep "image:" | uniq
    image: quay.io/coreos/etcd:v3.1.0-alpha.1
```

Check the other two pods and you should see the same result.


[k8s-home]: http://kubernetes.io
