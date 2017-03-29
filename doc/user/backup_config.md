# Backup Options Config Guide

In etcd operator, we provide the following options to save cluster backups to:
- Persistent Volume (PV) on GCE or AWS
- S3 bucket on AWS

This docs talks about how to configure etcd operator to use these backup options.

## PV on GCE

By default, operator supports saving backup to PV on GCE.
This is done by passing flag `--pv-provisioner=kubernetes.io/gce-pd` to operator, which is also the default value.
This is essentially saving backups to an instance of GCE PD.

## PV on AWS

If running on AWS Kubernetes, pass the flag `--pv-provisioner=kubernetes.io/aws-ebs` to operator.
See [AWS deployment](../../example/deployment-aws.yaml).
This is essentially saving backups on an instance of AWS EBS.

## S3 on AWS

Saving backups to S3 is also supported. The following flags need to be passed to operator:
- `backup-aws-secret`: The name of the kube secret object that stores the AWS credential file. The file name must be 'credentials'.
Profile must be "default".
- `backup-aws-config`: The name of the kube configmap object that stores the AWS config file. The file name must be 'config'.
For example, create configmap as `kubectl create configmap aws --from-file=$AWS_DIR/config`, and then set flag `--backup-aws-config=aws`.
- `backup-s3-bucket`: The name of the S3 bucket to store backups in.

For example, let's say we have aws credentials:
```
$ cat ~/.aws/credentials
[default]
aws_access_key_id = XXX
aws_secret_access_key = XXX
```

We create a secret "aws":
```
$ kubectl create secret generic aws --from-file=$AWS_DIR/credentials
```

We have aws config:
```
$ cat ~/.aws/config
[default]
region = us-west-1
```

We create a configmap "aws":
```
$ kubectl create configmap aws --from-file=$AWS_DIR/config
```

What we have:
- a secret "aws";
- a configmap "aws";
- S3 bucket "etcd_backups";

We will start etcd operator with the following flags:
```
$ ./etcd-operator ... --backup-aws-secret=aws --backup-aws-config=aws --backup-s3-bucket=etcd_backups
```
Then we could start using S3 storage for backups. See [spec examples](spec_examples.md) .
