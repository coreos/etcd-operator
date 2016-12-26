# Resource Ownership and GC

## Resource Ownership

An etcd cluster creates and manages pods, services and replica set. Those resources are owned by the cluster.
Two cluster owns completely disjoint set of resources, even if they have the same name.
For example, etcd cluster "A" was deleted and later etcd cluster "A" was created again; we think these are two different clusters.
Thus, the new cluster should not mange the resources from the old one. The old resources should be treated as garbage and to be collected.

As discussed in https://github.com/coreos/etcd-operator/issues/517,
we correlate owner (i.e. cluster) and its resources by making use of `ObjectMeta.OwnerReferences` field.

For etcd pods and services, they will have only one owner -- its managing cluster.

## GC

Github issue: https://github.com/coreos/etcd-operator/issues/518

We will talk about two strategies to do GC in the following.
We are only covering etcd pods, services, although the algorithm applies to more resources.

### Lazy deletion

We should remove resources when deleting or creating a cluster.
We remove them via label convention:
- remove pods with selector label{"etcd_cluster": $cluster_name, "app": "etcd"}
- remove services with selector label{"etcd_cluster": $cluster_name}

As a side note on creating a cluster:
Right before creating a cluster, if a pod or svc was selected out, then it must hangs around as garbage.
This could lead to confusing, even harmful cases.
So we must delete all related garbage resources before we start the new cluster.


### Periodic full GC

Every interval (10 mins or so), we find out orphaned pods/svcs.

Initially, our use case would be O(10) clusters, and thus total selected items would be at most O(100).

A simple full scanning algorithm:
- List pods, services.
- Controller have the knowledge of etcd clusters.
- Find out pods, services whose OwnerRef-ed cluster doesn't exist in controller knowledge.
