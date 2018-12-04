# Backups using Google Cloud Storage (GCS)

Etcd backup operator backs up the data of an etcd cluster running on Kubernetes to a remote storage such as Google Cloud Storage (GCS). If it is not deployed yet, please follow [here](walkthrough/backup-operator.md#deploy-etcd-backup-operator) to deploy it, e.g. by running

```sh
kubectl apply -f example/etcd-backup-operator/deployment.yaml
```

## Setup Google Cloud Platform (GCP) backup account, GCS bucket, and Secret

  ```bash
  # create account
  BACKUP_ACCOUNT='backup-creator'
  BACKUP_ACCOUNT_EMAIL="${BACKUP_ACCOUNT}@$(gcloud config get-value project).iam.gserviceaccount.com"
  gcloud iam service-accounts create "$BACKUP_ACCOUNT" --display-name='Backup creator service account'

  # create bucket and set permissions
  BACKUP_BUCKET='mygcsbackupsbucket'
  gsutil mb "$BACKUP_BUCKET"
  gsutil iam ch "serviceAccount:${BACKUP_ACCOUNT_EMAIL}:objectCreator" "$BACKUP_BUCKET"

  # create secret
  kubectl create secret generic gcp-credentials \
      --from-literal="credentials.json=$(gcloud iam service-accounts keys create - --iam-account="$BACKUP_ACCOUNT_EMAIL")"
  ```

## Create EtcdBackup CR

Create an `EtcdBackup` CR file `etcdbackup.yaml` which uses secret `gcp-credentials` from the previous section:

```yaml
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdBackup
metadata:
  name: example-etcd-cluster-backup
spec:
  etcdEndpoints: ["http://example-etcd-cluster-client:2379"]
  storageType: GCS
  gcs:
    # The format of the path must be: "<gcs-bucket-name>/<path-to-backup-object>"
    path: my-etcd-backups-bucket/my-etcd-backup-object
    gcpSecret: gcp-credentials
```

Apply it to kubernetes cluster:

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
  gcs:
    gcpSecret: gcp-credentials
    path: my-etcd-backups-bucket/my-etcd-backup-object
  etcdEndpoints:
  - http://example-etcd-cluster-client:2379
  storageType: GCS
status:
  etcdRevision: 1
  etcdVersion: 3.2.13
  succeeded: true

```

We should see the backup files after running the following command:

```bash
$ gsutil ls gs://my-etcd-backups-bucket/my-etcd-backup-object
gs://my-etcd-backups-bucket/my-etcd-backup-object
```

Alternatively, you can also login into the Google Cloud Platform Console to view these backups.

## Create EtcdRestore CR

Etcd restore operator is in charge of restoring etcd cluster from backup. If it is not deployed, please deploy by following command:

```yaml
kubectl apply -f example/etcd-restore-operator/deployment.yaml
```

Now kill all the etcd pods to simulate a cluster failure:

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
  backupStorageType: GCS
  gcs:
    # The format of the path must be: "<gcs-bucket-name>/<path-to-backup-file>"
    path: my-etcd-backups-bucket/my-etcd-backup-object
    gcpSecret: gcp-credentials
```

Check the `status` section of the `EtcdRestore` CR:

```sh
$ kubectl get etcdrestore example-etcd-cluster -o yaml
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdRestore
...
spec:
  gcs:
    gcpSecret: gcp-credentials
    path: my-etcd-backups-bucket/my-etcd-backup-object
  backupStorageType: GCS
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
