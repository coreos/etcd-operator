# Backups using Azure Blob Service (ABS)

Etcd backup operator backups the data of an etcd cluster running on Kubernetes to a remote storage such as Azure Blob Service (ABS). If it is not deployed yet, please follow [here](walkthrough/backup-operator.md#deploy-etcd-backup-operator) to deploy it, e.g. by running

```sh
kubectl apply -f example/etcd-backup-operator/deployment.yaml
```

## Setup Azure Secret

Create a secret file `abs-secret.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: abs-credentials
  namespace: etcd
type: Opaque
stringData:
  storage-account: "<storage-account>"
  storage-key: "<storage-key>"
  cloud: "<cloud>"
```

In above secret, following data should be provided:

- `storage-account`: the name of Azure storage account.
- `storage-key`: the storage key of storage account.
- `cloud`: the name of cloud environment, e.g. `AzurePublicCloud` or `AzureChinaCloud`. Optional. If not specified, it is defaulted to `AzurePublicCloud`.

And then create the secret:

```sh
kubectl apply -f abs-secret.yaml
```

## Create EtcdBackup CR

First, create a container in the storage account:

```sh
az storage container create -n etcd-backups --account-name <storage-account> --account-key <storage-key>
```

Then create a `EtcdBackup` CR file `etcdbackup.yaml` which uses secret `abs-credentials`:

```yaml
apiVersion: "etcd.database.coreos.com/v1beta2"
kind: "EtcdBackup"
metadata:
  name: example-etcd-cluster-backup
  namespace: etcd
spec:
  etcdEndpoints: ["http://example-etcd-cluster-client:2379"]
  storageType: ABS
  abs:
    # The format of the path must be: "<abs-container-name>/<path-to-backup-file>"
    path: etcd-backups/etcd.backup
    absSecret: abs-credentials
```

Finally apply it to kubernetes cluster:

```sh
kubectl apply -f etcdbackup.yaml
```

Check the `status` section of the `EtcdBackup` CR:

```sh
$ kubectl get EtcdBackup example-etcd-cluster-backup -o yaml
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdBackup
...
spec:
  abs:
    absSecret: abs-credentials
    path: etcd-backups/etcd.backup
  etcdEndpoints:
  - http://example-etcd-cluster-client:2379
  storageType: ABS
status:
  etcdRevision: 1
  etcdVersion: 3.2.13
  succeeded: true

```

We should see the backup files after running the following command:

```bash
$ az storage blob list -c etcd-backups --account-name <storage-account> --account-key <storage-key>
Name         Blob Type    Blob Tier      Length  Content Type              Last Modified              Snapshot
-----------  -----------  -----------  --------  ------------------------  -------------------------  ----------
etcd.backup  BlockBlob                    24608  application/octet-stream  2018-08-13T05:42:03+00:00
```

Alternatively, you can also login into the Azure Portal to view these backups.

## Create EtcdRestore CR

Etcd restore operator is in charge of restoring etcd cluster from backup. If it is not deployed, please deploy by following command:

```yaml
kubectl apply -f example/etcd-restore-operator/deployment.yaml
```

Now kill all the etcd pods to simulate disaster failure:

```sh
kubectl delete pod -l app=etcd,etcd_cluster=example-etcd-cluster --force --grace-period=0
```

Create EtcdRestore CR:

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
  backupStorageType: ABS
  abs:
    # The format of the path must be: "<abs-container-name>/<path-to-backup-file>"
    path: etcd-backups/etcd.backup
    absSecret: abs-credentials
```

Check the `status` section of the `EtcdRestore` CR:

```sh
$ kubectl get etcdrestore example-etcd-cluster -o yaml
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdRestore
...
spec:
  abs:
    absSecret: abs-credentials
    path: etcd-backups/etcd.backup
  backupStorageType: ABS
  etcdCluster:
    name: example-etcd-cluster
status:
  succeeded: true
```

Verify the `EtcdCluster` CR and restored pods for the restored cluster:

```sh
$ kubectl get etcdcluster
NAME                   AGE
example-etcd-cluster   1m

$ kubectl get pods -l app=etcd,etcd_cluster=example-etcd-cluster
NAME                                     READY     STATUS    RESTARTS   AGE
example-etcd-cluster-795649v9kq          1/1       Running   1          3m
example-etcd-cluster-jtp447ggnq          1/1       Running   1          4m
example-etcd-cluster-psw7sf2hhr          1/1       Running   1          4m
```
