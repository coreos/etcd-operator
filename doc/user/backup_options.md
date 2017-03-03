# Backup Options Setup Guide

In etcd operator, we provide the following options to save cluster backups to:
- Persistent Volume (PV) on GCE or AWS
- S3 bucket on AWS

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
For example, create secret as `kubectl create secret generic aws --from-file=$AWS_DIR/config`, and then set flag `--backup-aws-secret=aws`.
- `backup-aws-config`: The name of the kube configmap object that stores the AWS config file. The file name must be 'config'.
For example, create configmap as `kubectl create configmap aws --from-file=$AWS_DIR/config`, and then set flag `--backup-aws-config=aws`.
- `backup-s3-bucket`: The name of the S3 bucket to store backups in.
