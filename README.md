# kube-etcd-controller

## Initialize the TPR 

(TODO: auto create TPR when deploy the controller)

```bash
cat example/etcd-clusters-tpr.yaml
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
kubectl create -f example/etcd-clusters-tpr.yaml

kubectl get thirdpartyresources
NAME                      DESCRIPTION             VERSION(S)
etcd-cluster.coreos.com   Managed etcd clusters   v1,v2
```


## Deploy kube-etcd-controller

(TODO: deploy it using Kubernetes...)

```bash
./kube-etcd-controller --master=http://localhost:8080
```

## Create an etcd cluster

```bash
cat example/example-etcd-cluster.yaml
```

```yaml
apiVersion: "coreos.com/v1"
kind: "EtcdCluster"
metadata:
  name: "example-etcd-cluster"
size: 3
```

```bash
kubectl create -f example/example-etcd-cluster.yaml
```

```bash
kubectl get pods
```

```bash
kubectl get services
```

```bash
kubectl log etcd0-[uuid]
```
