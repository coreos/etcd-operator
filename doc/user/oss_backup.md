# Backups using Alibaba Cloud Object Storage Service (OSS)

Etcd backup operator backs up the data of an etcd cluster running on Kubernetes to a remote storage such as Alibaba Cloud Object Storage Service (OSS). If it is not deployed yet, please follow the [instructions](walkthrough/backup-operator.md#deploy-etcd-backup-operator) to deploy it, e.g. by running

```sh
kubectl apply -f example/etcd-backup-operator/deployment.yaml
```

## Setup Alibaba Cloud backup account, OSS bucket, and Secret

1. Login [Alibaba Cloud Console](https://www.alibabacloud.com) (or [Aliyun Console](https://www.aliyun.com/) if you are in China) and create your own [AccessKey](https://www.alibabacloud.com/help/doc-detail/29009.htm) which gives you the AccessKeyID (AKID) and AccessKeySecret (AKS). You can optionally create an Object Storage Service ([OSS](https://www.alibabacloud.com/help/doc-detail/31947.htm)) bucket for backups.
2. Create a secret storing your AKID and AKS in Kubernetes.  

 ```yaml  
apiVersion: v1
  kind: Secret
  metadata:
    name: my-oss-credentials
  type: Opaque
  data:
    accessKeyID: <my-access-key-id>
    accessKeySecret: <my-access-key-secret>
 ```  

3. Create an `EtcdBackup` CR file `etcdbackup.yaml` which uses secret `my-oss-credentials` from the previous step.  
```yaml
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdBackup
metadata:
  name: etcd-cluster-with-oss-backup
spec:
  backupPolicy:
    ...
  etcdEndpoints:
    - "http://example-etcd-cluster-client:2379"
  storageType: OSS
  oss:
    endpoint: http://oss-cn-hangzhou.aliyuncs.com
    ossSecret: my-oss-credentials
    path: my-etcd-backups-bucket/etcd.backup
```   

4. Apply yaml file to kubernetes cluster.  
```sh
kubectl apply -f etcdbackup.yaml
```
5. Check the `status` section of the `EtcdBackup` CR.
```console
$ kubectl get EtcdBackup etcd-cluster-with-oss-backup -o yaml
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdBackup
...
spec:
  oss:
    ossSecret: my-oss-credentials
    path: my-etcd-backups-bucket/etcd.backup
    endpoint: http://oss-cn-hangzhou.aliyuncs.com
  etcdEndpoints:
  - http://example-etcd-cluster-client:2379
  storageType: OSS
status:
  etcdRevision: 1
  etcdVersion: 3.2.13
  succeeded: true
```

6. We should see the backup files from Alibaba Cloud OSS Console.


## Restore etcd based on data from OSS.

Etcd restore operator is in charge of restoring etcd cluster from backup. If it is not deployed, please deploy by following command:

```sh
kubectl apply -f example/etcd-restore-operator/deployment.yaml
```

Now kill all the etcd pods to simulate a cluster failure:

```sh
kubectl delete pod -l app=etcd,etcd_cluster=example-etcd-cluster --force --grace-period=0
```

1. Create an EtcdRestore CR.
```yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdRestore"
metadata:
  # The restore CR name must be the same as spec.etcdCluster.name
  name: example-etcd-cluster
spec:
  etcdCluster:
    # The namespace is the same as this EtcdRestore CR
    name: example-etcd-cluster
  backupStorageType: OSS
  oss:
    # The format of the path must be: "<oss-bucket-name>/<path-to-backup-file>"
    path: my-etcd-backups-bucket/etcd.backup
    ossSecret: my-oss-credentials
    endpoint: http://oss-cn-hangzhou.aliyuncs.com
```

2. Check the `status` section of the `EtcdRestore` CR.     
```sh
$ kubectl get etcdrestore example-etcd-cluster -o yaml
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdRestore
...
spec:
  oss:
    ossSecret: my-oss-credentials
    path: my-etcd-backups-bucket/etcd.backup
    endpoint: http://oss-cn-hangzhou.aliyuncs.com
  backupStorageType: OSS
  etcdCluster:
    name: example-etcd-cluster
status:
  succeeded: true
```

3. Verify the `EtcdCluster` CR and restored pods for the restored cluster.    
```sh  
$ kubectl get etcdcluster
$ kubectl get pods -l app=etcd,etcd_cluster=example-etcd-cluster
```
