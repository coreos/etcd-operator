# etcd restore operator

## Overview

The etcd-restore-operator can restore an etcd cluster on Kubernetes from backup.

The overall workflow is:
- Create the etcd-restore-operator
- Create an `EtcdRestore` Custom Resource which triggers a restore request that specifies:
  - the etcd cluster spec
  - how to access the backup
- The etcd-restore-operator will restore a new cluster from the backup
- The etcd-operator takes over the management of the restored cluster

Note that currently the etcd-restore-operator only supports restoring from backups saved on S3.

**Prerequisite**
- Setup RBAC and deploy an etcd operator. See [Install Guide][install_guide]
- Have an etcd backup saved on S3. See the [etcd-backup-operator README][backup-operator-README] as one way to save a backup to S3.

>Note: This demo uses the `default` namespace.


### Simulate disaster failure

1. Make sure `example-etcd-cluster` EtcdCluster CR exists

    ```sh
    kubectl get etcdcluster example-etcd-cluster
    ```

2. Kill etcd pods to simulate disaster failure

    ```sh
    kubectl delete pod -l app=etcd,etcd_cluster=example-etcd-cluster --force --grace-period=0
    ```

### Deploy the etcd-restore-operator

1. Create a deployment of the etcd-restore-operator:

    ```sh
    kubectl create -f example/etcd-restore-operator/deployment.yaml
    ```

2. Verify the following resources exist:

    ```sh
    $ kubectl get pods
    NAME                                     READY     STATUS    RESTARTS   AGE
    etcd-restore-operator-4203122180-npn3g   1/1       Running   0          7s
    ```

3. Verify that the etcd-restore-operator creates the `EtcdRestore` CRD:

    ```sh
    $ kubectl get crd
    NAME                                       KIND
    etcdrestores.etcd.database.coreos.com      CustomResourceDefinition.v1beta1.apiextensions.k8s.io
    ```

### Setup AWS Secret

Create a Kubernetes secret that contains AWS credentials and config. This is used by the etcd-restore-operator to retrieve the backup from S3.

1. Verify that the local aws config and credentials files exist:

    ```sh
    $ cat $AWS_DIR/credentials
    [default]
    aws_access_key_id = XXX
    aws_secret_access_key = XXX

    $ cat $AWS_DIR/config
    [default]
    region = <region>
    ```

2. Create the secret `aws`:

    ```sh
    kubectl create secret generic aws --from-file=$AWS_DIR/credentials --from-file=$AWS_DIR/config
    ```

### Create EtcdRestore CR

Create the `EtcdRestore` CR:

>Note: This example uses k8s secret "aws" and S3 path "mybucket/etcd.backup"

```sh
sed -e 's|<full-s3-path>|mybucket/etcd.backup|g' \
    -e 's|<aws-secret>|aws|g' \
    example/etcd-restore-operator/restore_cr.yaml \
    | kubectl create -f -
```

### Verify the CR status and restored cluster

1. Check the `status` section of the `EtcdRestore` CR:

    ```sh
    $ kubectl get etcdrestore example-etcd-cluster -o yaml
    apiVersion: etcd.database.coreos.com/v1beta2
    kind: EtcdRestore
    ...
    status:
      succeeded: true
    ```

2. Verify the `EtcdCluster` CR for the restored cluster:

    ```
    $ kubectl get etcdcluster
    NAME                    KIND
    example-etcd-cluster   EtcdCluster.v1beta2.etcd.database.coreos.com
    ```

3. Verify that the etcd-operator scales the cluster to the desired size:

    ```sh
    $ kubectl get pods
    NAME                                     READY     STATUS    RESTARTS   AGE
    etcd-operator-2486363115-ltc17           1/1       Running   0          1h
    etcd-restore-operator-4203122180-npn3g   1/1       Running   0          30m
    example-etcd-cluster-795649v9kq          1/1       Running   1          3m
    example-etcd-cluster-jtp447ggnq          1/1       Running   1          4m
    example-etcd-cluster-psw7sf2hhr          1/1       Running   1          4m
    ```

### Cleanup

Delete the etcd-restore-operator deployment and service, and the `EtcdRestore` CR. 
>Note: Deleting the `EtcdRestore` CR won't delete the `EtcdCluster` CR.

```sh
kubectl delete etcdrestore example-etcd-cluster
kubectl delete -f example/etcd-restore-operator/deployment.yaml
```


[backup-operator-README]:./backup-operator.md
[install_guide]:../install_guide.md
