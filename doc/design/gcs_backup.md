# Store Backup in GCS

We want to have an option to store backup in Google Cloud Storage, or GCS.

## Prerequisites

Create and download service account JSON key from https://console.cloud.google.com/apis/credentials.

## Flags of etcd operator

When staring etcd operator, we need to provide flags in order to access Google Cloud Platform, or GCP:

```bash
$ etcd-operator \
  --backup-gcp-secret ${secret_name} \
  --backup-gcp-config ${configmap_name} \
  --backup-gcs-bucket ${bucket_name} \
  ...
```

Let's explain them one by one:

- `--backup-gcp-secret` takes the name of the Kubernetes [secret](https://kubernetes.io/docs/user-guide/kubectl/v1.6/#-em-secret-generic-em-) object that stores the Google Cloud Platform service account credential JSON file.

To create the Kubernetes secret object:

```bash
$ kubectl create secret generic gcp-credential --from-file=${gcp_credential_file}
```

`gcp-credential` is the ${secret_name}. `gcp_credential_file` is the file path of Google Cloud Platform service account JSON key.

An example credential file:

```json
{
  "type": "service_account",
  "project_id": "YOUR_PROJECT",
  "private_key_id": "test",
  "private_key": "-----BEGIN PRIVATE...END PRIVATE KEY-----\n",
  "client_email": "test@developer.gserviceaccount.com",
  "client_id": "111",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://accounts.google.com/o/oauth2/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/test.gserviceaccount.com"
}
```

- `--backup-gcp-config` takes the name of the Kubernetes [`configmap`](https://kubernetes.io/docs/user-guide/kubectl/v1.6/#-em-configmap-em-) object that presents the GCP service account JSON key file.

To create a `configmap`:

```bash
$ kubectl create configmap gcp-config --from-file=${gcp_credential_file}
```

We only use "Multi-Regional" storage by default, so there is no need to set region. See [Google Cloud Storage Classes](https://cloud.google.com/storage/docs/storage-classes) for more detail.

- `--backup-gcs-bucket` takes the name of the Google Cloud Storage bucket in which the operator stores backup.

The backups of each cluster are saved in individual directories under given bucket. The format would look like: *bucket_name/v1/cluster_name/* .

For example, given bucket "etcd-backups" and if operator has saved backups of cluster "etcd-A", we should see backup files running commands:

```bash
$ gsutil ls gs://etcd-backups/v1/etcd-A/
```

Or just login into Google Cloud Storage console to view it.

## How to create a cluster with backup in GCS

When we create a cluster with backup, we set the backup.storageType to "GCS".
For example, a yaml file would look like:

```yaml
apiVersion: "etcd.coreos.com/v1beta1"
kind: "Cluster"
metadata:
  name: "etcd-cluster-with-backup"
spec:
  ...
  backup:
    ...
    storageType: "GCS"
```
