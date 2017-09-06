# Backup Options Config Guide

In etcd operator, we provide the following options to save cluster backups to:
- Persistent Volume (PV) on GCE or AWS
- Persistent Volume (PV) with custom StorageClasses
- S3 bucket on AWS
- Azure Blob Storage (ABS) container

This docs talks about how to configure etcd operator to use these backup options.

## PV with custom StorageClass

If your Kubernetes supports the [StorageClass](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#storageclasses) resource, you can use them to back up your etcd cluster. To do this, specify a `StorageClass` value in the cluster's Backup spec, like so:

```yaml
spec:
  ...
  backup:
    ...
    storageType: "PersistentVolume"
    pv:
      volumeSizeInMB: 512
      storageClass: foo
```

This spec field provides more granular control over how to persist etcd data to PersistentVolumes. This is essentially saving backups to a PersistentVolume with a predefined StorageClass.

## PV on GCE

**Note: It is recommended to use the StorageClass spec field because --pv-provisioner will be deprecated in a future release**

By default, operator supports saving backup to PV on GCE.
This is done by passing flag `--pv-provisioner=kubernetes.io/gce-pd` to operator, which is also the default value.
This is essentially saving backups to an instance of GCE PD.

## PV on AWS

**Note: It is recommended to use the StorageClass spec field because --pv-provisioner will be deprecated in a future release**

If running on AWS Kubernetes, pass the flag `--pv-provisioner=kubernetes.io/aws-ebs` to operator.
See [AWS deployment](../../example/deployment-aws.yaml).
This is essentially saving backups on an instance of AWS EBS.

## S3 on AWS

Saving backups to S3 is also supported. The S3 backup policy can be set at two levels:
- **operator level:** The same S3 configurations (bucket and secret names) will be used for all S3 backup enabled clusters created by the operator
- **cluster level:** Each cluster can specify its own S3 configuration.

If configurations for both levels are specified then the cluster level configuration will override the operator level configuration.

### Operator level configuration  

See the [S3 backup deployment](../../example/deployment-s3-backup.yaml.template) template on how to configure the operator to enable S3 backups. The following flags need to be passed to operator:
- `backup-aws-secret`: The name of the kube secret object that stores the AWS credential file. The file name must be 'credentials'.
Profile must be "default".
- `backup-aws-config`: The name of the kube configmap object that stores the AWS config file. The file name must be 'config'.
- `backup-s3-bucket`: The name of the S3 bucket to store backups in.

Both the secret and configmap objects must be created in the same namespace that the etcd-operator is running in.

For example, let's say we have aws credentials:
```
$ cat ~/.aws/credentials
[default]
aws_access_key_id = XXX
aws_secret_access_key = XXX
```

We create a secret "aws":
```
$ kubectl -n <namespace-name> create secret generic aws --from-file=$AWS_DIR/credentials
```

We have aws config:
```
$ cat ~/.aws/config
[default]
region = us-west-1
```

We create a configmap "aws":
```
$ kubectl -n <namespace-name> create configmap aws --from-file=$AWS_DIR/config
```

What we have:
- a secret "aws";
- a configmap "aws";
- S3 bucket "etcd_backups";

We will start etcd operator with the following flags:
```
$ ./etcd-operator ... --backup-aws-secret=aws --backup-aws-config=aws --backup-s3-bucket=etcd_backups
```
Then we could start using S3 storage for backups. See [spec examples](spec_examples.md#three-members-cluster-with-s3-backup) on how to configure a cluster that uses an S3 bucket as its storage type.

### Cluster level configuration

See the [S3 backup with cluster specific configuration](https://github.com/coreos/etcd-operator/blob/master/doc/user/spec_examples.md#s3-backup-and-cluster-specific-s3-configuration) spec to see what the cluster's `spec.backup` field should be configured as to set a cluster specific S3 backup configuration. The following additional fields need to be set under the cluster spec's `spec.backup.s3` field:
- `s3Bucket`: The name of the S3 bucket to store backups in.
- `awsSecret`: The secret object name which should contain two files named `credentials` and `config` .
- `prefix`: (Optional) The S3 [prefix](http://docs.aws.amazon.com/AmazonS3/latest/dev/ListingKeysHierarchy.html).

The profile to use in both the files `credentials` and `config` is `default` :
```
$ cat ~/.aws/credentials
[default]
aws_access_key_id = XXX
aws_secret_access_key = XXX

$ cat ~/.aws/config
[default]
region = us-west-1
```

We can then create the secret named "aws" from the two files by:
```bash
$ kubectl -n <namespace-name> create secret generic aws --from-file=$AWS_DIR/credentials --from-file=$AWS_DIR/config
```

Once the secret is created, it can be used to configure a new cluster or update an existing one with the specific S3 configurations:
```
spec:
  backup:
    s3:
      s3Bucket: example-s3-bucket
      awsSecret: aws
      prefix: example-prefix
```

For AWS k8s users: If `credentials` file is not given,
operator and backup sidecar pods will make use of AWS IAM roles on the nodes where they are deployed.

## ABS on Azure

The ABS backup policy is configured in a cluster's spec.  See [spec_examples.md](spec_examples.md#three-member-cluster-with-abs-backup) for an example.

### Prerequisites

  * An ABS container will need to be created in Azure. Here we name the container `etcd-backups`.

    ```
    $ export AZURE_STORAGE_ACCOUNT=<storage-account-name> AZURE_STORAGE_KEY=<storage-key>
    $ az storage container create -n etcd-backups
    ```

  * A Kubernetes secret will need to be created.

      An example of the secret manifest should look like:
      ```
      apiVersion: v1
      kind: Secret
      metadata:
        name: abs-credentials
      type: Opaque
      stringData:
        storage-account: <storage-account-name>
        storage-key: <storage-key>
      ```

      To create the secret from the secret manifest:
      ```
      $ kubectl -n <namespace> create -f secret-abs-credentials.yaml
      ```

What we have:
- A secret "abs-credentials"
- An ABS container "etcd_backups"

### Cluster configuration

The following fields need to be set under the cluster spec's `spec.backup.abs` field:
- `absContainer`: The name of the ABS container to store backups in.
- `absSecret`: The secret object name (as created above.)

An example cluster with specific ABS configurations then looks like:
```
spec:
  backup:
    storageType: "ABS"
    abs:
      absContainer: etcd-backups
      absSecret: abs-credentials
```


