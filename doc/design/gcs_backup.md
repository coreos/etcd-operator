# Backups using Google Cloud Storage (GCS)

## Cluster configured with GCS backup

To create a cluster with backups to GCS, set `backup.storageType` to `"GCS"`, supply the path (in the format "<gcs-bucket-name>/<path-to-backup-object>") in `gcs.path` and provide the Kubernetes secret storing the Google Cloud Platform account credentials to `gcs.gcpSecret`.  The bucket and secret must exist prior to backup creation.

An example cluster manifest would look like:

```bash
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdCluster
metadata:
  name: etcd-cluster-with-gcs-backup
spec:
  ...
  backup:
    ...
    storageType: GCS
    gcs:
      path: my-etcd-backups-bucket/my-etcd-backup-object
      gcpSecret: gcp-credentials

``` 

### In Detail:

- `"gcpSecret"` represents the name of the Kubernetes secret object that stores the credentials needed for GCP authorization, namely an authorization token or JSON credentials.

  The Kubernetes secret manifest looks like either:
  ```yaml
  apiVersion: v1
  kind: Secret
  metadata:
    name: gcp-credentials
  type: Opaque
  stringData:
    access-token: <token>
  ```

  Or:
  ```yaml
  apiVersion: v1
  kind: Secret
  metadata:
    name: gcp-credentials
  type: Opaque
  stringData:
    credentials.json: <JSON-credentials>
  ```

  Example of the latter:

  ```bash
  # create account
  BACKUP_ACCOUNT='backup-creator'
  BACKUP_ACCOUNT_EMAIL="${BACKUP_ACCOUNT}@$(gcloud config get-value project).iam.gserviceaccount.com"
  gcloud iam service-accounts create "$BACKUP_ACCOUNT" --display-name='Backup creator service account'

  # set permissions
  BACKUP_BUCKET='my-etcd-backups-bucket'
  gsutil mb "$BACKUP_BUCKET"
  gsutil iam ch "serviceAccount:${BACKUP_ACCOUNT_EMAIL}:objectCreator" "$BACKUP_BUCKET"

  # create secret
  kubectl create secret generic gcp-credentials \
      --from-literal="credentials.json=$(gcloud iam service-accounts keys create - --iam-account="$BACKUP_ACCOUNT_EMAIL")"
  ```
