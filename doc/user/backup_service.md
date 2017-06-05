## Backup service

A backup service will be created if the etcd cluster has backup enabled.
The backup service saves backup for the etcd cluster based on the requirement of the [backup spec](https://github.com/coreos/etcd-operator/blob/3ec1a1d38e0fc91a2e757ed322227c6816e5f110/example/example-etcd-cluster-with-backup.yaml#L8-L12).

The backup service will skip creating a new snapshot if the etcd cluster revision has not changed since the last snapshot, i.e the etcd-cluster data has not been modified (e.g., `Put`, `Delete`, `Txn`).

It also exposes an HTTP API for requesting a new backup and retrieving existing backups. The HTTP API can be accessed from inside the kubernetes cluster as:
```bash
$ curl "http://<cluster-name>-backup-sidecar:19999/v1/<command>
```

## HTTP API v1

#### GET /v1/backupnow

The backup service requests a backup from the etcd cluster immediately when it receives the `GET` request.

Response Body

JSON format of the backup status when backup is successful.

``` go
type BackupStatus struct {
    // Creation time of the backup.
    CreationTime string `json:"creationTime"`

    // Size is the size of the backup in MB.
    Size float64 `json:"size"`

    // Version is the version of the backup cluster.
    Version string `json:"version"`

    // TimeTookInSecond is the total time took to create the backup.
    TimeTookInSecond int `json:"timeTookInSecond"`
}
```

#### GET /v1/backup

The backup service returns the most recent backup in the body of the HTTP response when it receives the `GET` request.

Request Parameters

- etcdVersion (optional): backup service checks the compatibility between the latest backup and the etcd server with passed in `etcdVersion`.
For example, if we want to get a backup for etcd server 3.1.0, we should set etcdVersion to 3.1.0. Backup service will check the if its latest backup can be used to restore a 3.1.0 etcd cluster.

- etcdRevision (optional): when both etcdVersion and etcdRevision are provided, a backup with revision equal to etcdRevision and taken from the etcd cluster with version equal to etcdVersion will be returned. 

Response Headers

- X-etcd-Version: the etcd cluster version tht the backup was made from
- X-Revision: the etcd store revision when the backup was made

#### GET /v1/status

The backup service returns the service status in JSON format. The JSON payload is defined in pkg backapi.ServiceStatus.
