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

(TODO: deploy it using Kubernetes...)

```bash
$ ./kube-etcd-controller --master=http://localhost:8080
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
NAME                                         READY     STATUS    RESTARTS   AGE
etcd0-78b868e0-0a3e-4213-8299-988a1f828ee1   1/1       Running   0          5s
etcd1-78b868e0-0a3e-4213-8299-988a1f828ee1   1/1       Running   0          5s
etcd2-78b868e0-0a3e-4213-8299-988a1f828ee1   1/1       Running   0          5s
```

```bash
$ kubectl get services
NAME                                         CLUSTER-IP    EXTERNAL-IP   PORT(S)    AGE
etcd0-78b868e0-0a3e-4213-8299-988a1f828ee1   10.0.125.56   <none>        2380/TCP   49s
etcd1-78b868e0-0a3e-4213-8299-988a1f828ee1   10.0.189.13   <none>        2380/TCP   49s
etcd2-78b868e0-0a3e-4213-8299-988a1f828ee1   10.0.87.110   <none>        2380/TCP   49s
kubernetes                                   10.0.0.1      <none>        443/TCP    8m
```

```bash
$ kubectl log etcd0-78b868e0-0a3e-4213-8299-988a1f828ee1
...
2016-08-05 00:33:32.453768 I | api: enabled capabilities for version 3.0
2016-08-05 00:33:32.454178 N | etcdmain: serving insecure client requests on 0.0.0.0:2379, this is strongly discouraged!
```
