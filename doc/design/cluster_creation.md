# Cluster creation

The create a cluster when it receives an `added` event.

To initialize a cluster, we first need to create a seed etcd member. Then we can rely on resize to increase the cluster size to the desired number of members.

1. receive added event
2. create a member with setting --initial-cluster-state to new with a random cluster token
3. END.

## Failure recovery

If operator fails before creating the first member, we will end up with a cluster with no running pods.

Recovery can be challenging if this happens. It is impossible for us to differentiate a dead cluster from a uninitialized cluster.

We choose the simplest solution here. We always consider a cluster with no running pods a failed cluster. The operator will
try to recover it from existing backups. If there is no backup, we mark the cluster dead.
