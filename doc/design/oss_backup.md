# Backups using Alibaba Cloud Object Storage Service (OSS)

## Cluster configured with OSS backup

To create a backup in OSS, set `backup.storageType` to `"OSS"`, supply the path (in the format `<oss-bucket-name>/<path-to-backup-object>`) in `oss.path` and provide the Kubernetes secret storing the Alibaba Cloud account credentials to `oss.ossSecret`.  The secret must exist prior to backup creation. Etcd backup operator will create the bucket and object if not found. The field `oss.endpoint` is the target OSS service endpoint where the data is backed up. By default, `http://oss-cn-hangzhou.aliyuncs.com` will be used. If you want to back up the data to other regions, please specify another endpoint from [the list of region endpoints](https://www.alibabacloud.com/help/doc-detail/31837.htm).


An example backup manifest would look like:

```yaml
apiVersion: etcd.database.coreos.com/v1beta2
kind: EtcdBackup
metadata:
  name: etcd-cluster-with-oss-backup
  namespace: my-namespace
spec:
  backupPolicy:
    ...
  etcdEndpoints:
    - "http://example-etcd-cluster-client:2379"
  storageType: OSS
  oss:
    endpoint: http://oss-cn-hangzhou.aliyuncs.com
    ossSecret: my-oss-credentials
    path: my-etcd-backups-bucket/etcd.backup
```

### In Detail:

- `"ossSecret"` represents the name of the Kubernetes secret object that stores the credentials needed for Alibaba Cloud authorization, namely an authorization token.

  The Kubernetes secret manifest must have the following format:
  ```yaml
  apiVersion: v1
  kind: Secret
  metadata:
    name: my-oss-credentials
  type: Opaque
  data:
    accessKeyID: <my-access-key-id>
    accessKeySecret: <my-access-key-secret>
 ```
