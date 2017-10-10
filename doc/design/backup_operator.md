# etcd Backup Operator Design Document

## Problem

Currently, etcd-operator supports various backup strategies such as PV, S3, and Azure Blob Store. However, those strategies are embedded into the etcd operator code itself. Support for new backup strategy are required to be pushed into etcd-operator codebase. As more backup strategies are added, the etcd-operator code base would be hard to maintain. In Addition, users may want to implement their backup strategy without confining to the current etcd-operator’s backup spec and may not want to dependent on etcd-operator codebase for backup.

## Solution

To give user the flexibility of handling etcd backup his/her own way and to decouple backup logic out of etcd operator, a standalone etcd-backup operator can be used to manage the backup.

## A Proposed Architecture 

In order to control backup of etcd clusters, a standalone etcd backup operator can be used to listen for a customized backup CRD:

First, User creates a backup CRD that etcd backup operator will be watching on:

```yaml
apiVersion: apiextentions.k8s.io/v1beta2
kind: CustomResourceDefiniton
metadata:
  name: etcdbackups.etcd.database.coreos.com
spec:
  group: etcd.database.coreos.com
  version: v1beta2
  Scope: Namespaced
  names: 
    kind: EtcdBackup
    plural: etcdbackups
```

Then, start the etcd-backup-operator where it does the following:

* checks if CRD exists.
* watches for new backup CR. 
* manages backup according to `EtcdBackupSpec`.
* reports backup status. 

`EtcdBackupSpec` definition:

```go
type EtcdBackupSpec struct {
    // clusterName is the etcd cluster name that needs backup.
    ClusterName string `json:"clusterName,omitempty"`
    // StorageType is the type of backup storage.
    StorageType string `json:"storageType"`
    // StorageSource details storage sources.
    StorageSource `json:",inline"`
    // Any additional backup configs.
}
type StorageSource struct {
    // S3 contains resources that operator needs to store backup on S3.
    S3 *S3Source
}
type S3Source struct {
	// The name of the AWS S3 bucket to store backups in.
	// S3Bucket overwrites the default etcd operator wide bucket.
	S3Bucket string `json:"s3Bucket,omitempty"`

	// Prefix is the S3 prefix used to prefix the bucket path.
	// It's the prefix at the beginning.
	// After that, it will have version and cluster specific paths.
	Prefix string `json:"prefix,omitempty"`

	// The name of the secret object that stores the AWS credential and config files.
	// The file name of the credential MUST be 'credentials'.
	// The file name of the config MUST be 'config'.
	// The profile to use in both files will be 'default'.
	//
	// AWSSecret overwrites the default etcd operator wide AWS credential and config.
	AWSSecret string `json:"awsSecret,omitempty"`
}
```

