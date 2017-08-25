# Backups using Azure Blob Service (ABS)

In a very similar fashion to backups using [S3](./s3_backup.md), we want to have an option to store backups in ABS.

## Setup

For ABS backups, configuration is only set per etcd cluster, so no special flags are needed when starting the operator:

```bash
$ etcd-operator
```

## Cluster configured with ABS backup

When we create a cluster with backup, we set `backup.storageType` to `"ABS"`.  Additionally, the ABS container for holding backups and the Kubernetes secret storing the Azure Storage account credentials must be supplied.  Both of these resources must exist prior to cluster creation (as will be discussed further below).

For example, a yaml file would look like:

```bash
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdCluster"
metadata:
  name: "etcd-cluster-with-abs-backup"
spec:
  ...
  backup:
    ...
    storageType: "ABS"
    # Cluster-specific ABS credentials may be provided below
    abs:
      absContainer: "myabscontainer"
      absSecret: "abs-credentials"

``` 

### In Detail:

- `"absSecret"` represents the name of the Kubernetes secret object that stores the environment variables needed for ABS storage account authorization, namely, the storage account name and the storage account key.

  The Kubernetes secret manifest looks like:
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

  (Where the user is expected to provide the actual Azure Storage Account and Storage Key )

  To create the secret object from the manifest above:

  ```bash
  $ kubectl create -f secret-abs-credentials.yaml
  ```

- `"absContainer"` represents the name of the ABS container in which the operator will store backups.

  The backups of each cluster are saved in individual directories under the given container.

  As a reminder, to create a container via Azure's `az` command line, one may do the following:

  ```bash
  $ az storage container create -n etcd-backups
  ```

  The full directory/file format looks like: `*container_name/<prefix>/cluster_name/<backup file>`, where `<prefix>` includes version (`v1`) and the Kubernetes namespace the cluster lies in (`etcd-ns`) and `<backup file>` represents the backup filename.

  For example, given container "etcd-backups" and cluster "etcd-a", we should see the backup files after running the following command:

  ```bash
  $ az storage blob list -c etcd-backups
  Name                                                 Blob Type      Length  Content Type              Last Modified
  --------------------------------------------------   -----------  --------  ------------------------  -------------------------
  v1/etcd-ns/etcd-a/3.1.8_0000000000000326_etcd.backup BlockBlob      647200  application/octet-stream  2017-06-29T20:19:32+00:00
  ...
  ```

  Alternatively, one many login into the Azure Portal to view these backups.
