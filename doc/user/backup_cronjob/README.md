# Periodic etcd Backup using CronJob

This doc talks about how to use Kubernetes [CronJob][k8s_cronjob] to make periodic etcd backups.

Prerequisites:

- Kubernetes cluster and kubectl ready
- etcd-operator, etcd-backup-operator deployed
- `example-etcd-cluster` EtcdCluster CR is created and the cluster is running

All commands assume the current working directory to be the root directory of etcd-operator repo.
We are also following the example configurations in [backup-operator walkthrough][backup-operator-walkthrough].


## Backup ConfigMap

First we create a [ConfigMap](./configmap.yaml) containing the Backup CR template for the CronJob container to mount and reuse it.

```
kubectl create -f ./doc/user/backup_cronjob/configmap.yaml
```

## Backup CronJob

Then we create a [CronJob](./cronjob.yaml) which every 30 minutes would make a backup with name suffixed with the current timestamp.

```
kubectl create -f ./doc/user/backup_cronjob/cronjob.yaml
```

This example has the following limitations:

- **Backups and EtcdBackup CRs will keep being created**. Clean up/GC old resources properly.
- The CronJob pods use "default" namespace service account.
  In production case, replace it with a dedicated service account and restrict the permissions via RBAC.


[k8s_cronjob]:https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/
[backup-operator-walkthrough]:../walkthrough/backup-operator.md