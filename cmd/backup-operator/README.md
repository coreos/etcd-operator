# etcd backup operator

## Overview

etcd backup operator backups the data of a etcd cluster running on [Kubernetes][Kube] to a remote storage such as AWS [S3][s3].

## Getting Started

Try out etcd backup operator by running it on Kubernetes and then create a `EtcdBackup` Custom Resource which contains the targeting etcd cluster and S3 backup config; the etcd backup operator automatically picks up the `EtcdBackup` Custom Resource, retrieves etcd snapshot, and then saves it to S3.
>Note: The demo uses the `default` namespace.

Prerequisites: 
* access to a Kubernetes environment.
* see [Instruction][etcd_cluster_deploy] to deploy an etcd cluster. 

### Deploy etcd backup operator

Once `example-etcd-cluster` is running, let's create a backup for `example-etcd-cluster` using etcd backup operator. 

First, deploy an etcd backup operator:
> Note: etcd backup operator also creates EtcdBackup CRD when starting.

```sh
$ kubectl create -f example/etcd-backup-operator/deployment.yaml
$ kubectl get pod
NAME                                    READY     STATUS    RESTARTS   AGE
etcd-backup-operator-1102130733-hhgt7   1/1       Running   0          3s
```

### Create AWS Secret

Create a Kubernetes secret that contains aws config/credential; etcd backup operator uses the secret to gain access to S3 in order to save the etcd snapshot.

Verify that the local aws config and credentials files exist:
```sh
$ cat $AWS_DIR/credentials
[default]
aws_access_key_id = XXX
aws_secret_access_key = XXX

$ cat $AWS_DIR/config
[default]
region = <region>
```

Create kubernetes secret `aws`:

`kubectl create secret generic aws --from-file=$AWS_DIR/credentials --from-file=$AWS_DIR/config`

### Create Backup CR

Fill the template `example/etcd-backup-operator/backup_cr.yaml` with concrete `s3Bucket` and `awsSecret` values and trigger a backup:

```sh
sed -e 's/<s3-bucket-name>/mybucket/g' \
    -e 's/<aws-secret>/aws/g' \
    example/etcd-backup-operator/backup_cr.yaml \
    | kubectl create -f -
```

### Verify status

Verify the backup status by checking the `status` section of the `EtcdBackup` CR:
```
$ kubectl get EtcdBackup example-etcd-cluster -o yaml
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdBackup
...
status:
  s3Path: mybucket/v1/default/example-etcd-cluster/3.1.8_0000000000000001_etcd.backup
  succeeded: true
```

* `s3Path` is the full S3 object path to the stored etcd backup. 

This demonstrates etcd backup operator's basic one time backup functionality.


[Kube]:https://github.com/kubernetes/kubernetes
[s3]:https://aws.amazon.com/s3/
[etcd_cluster_deploy]:https://github.com/coreos/etcd-operator#create-and-destroy-an-etcd-cluster
[minikube]:https://github.com/kubernetes/minikube

