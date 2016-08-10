# kube-etcd-controller

## Initialize the TPR 

(TODO: auto create TPR when deploy the controller)

```bash
$ cat example/etcd-clusters-tpr.yaml
```

```yaml
apiVersion: extensions/v1beta1
kind: ThirdPartyResource
description: "Managed etcd clusters"
metadata:
  name: "etcd-cluster.coreos.com"
versions:
  - name: v1
  - name: v2
```

```bash
$ kubectl create -f example/etcd-clusters-tpr.yaml

$ kubectl get thirdpartyresources
NAME                      DESCRIPTION             VERSION(S)
etcd-cluster.coreos.com   Managed etcd clusters   v1,v2
```


## Deploy kube-etcd-controller

```bash
$ kubectl create -f example/etcd-controller.yaml
```

## Create an etcd cluster

```bash
$ cat example/example-etcd-cluster.yaml
```

```yaml
apiVersion: "coreos.com/v1"
kind: "EtcdCluster"
metadata:
  name: "example-etcd-cluster"
size: 3
```

```bash
$ kubectl create -f example/example-etcd-cluster.yaml
```

```bash
$ kubectl get pods
NAME                       READY     STATUS    RESTARTS   AGE
example-etcd-cluster-0000   1/1       Running   0          5s
example-etcd-cluster-0001   1/1       Running   0          5s
example-etcd-cluster-0002   1/1       Running   0          5s
```

```bash
$ kubectl get services
NAME                        CLUSTER-IP    EXTERNAL-IP   PORT(S)    AGE
example-etcd-cluster-0000   10.0.125.56   <none>        2380/TCP   49s
example-etcd-cluster-0001   10.0.189.13   <none>        2380/TCP   49s
example-etcd-cluster-0002   10.0.87.110   <none>        2380/TCP   49s
kubernetes                  10.0.0.1      <none>        443/TCP    8m
```

```bash
$ kubectl log example-etcd-cluster-0000
...
2016-08-05 00:33:32.453768 I | api: enabled capabilities for version 3.0
2016-08-05 00:33:32.454178 N | etcdmain: serving insecure client requests on 0.0.0.0:2379, this is strongly discouraged!
```

## Destory an existing etcd cluster

```bash
$ kubectl delete -f example/example-etcd-cluster.yaml
```

```bash
$ kubectl get pods
NAME                       READY     STATUS    RESTARTS   AGE
```
