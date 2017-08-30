# Use Persistent Volumes for etcd data

## Proposal and Motivation

Today etcd-operator creates ephemeral etcd members: in their pods the etcd data is stored inside a volume of type emptyDir and the pod restart policy is set to `Never`.
This has the following failure cases.

### Failure cases

Assume a N members etcd cluster with all members healthy

#### Case A

##### Event

One of these events happens:
* one etcd member process exits (processes crashes, killed etc...)
* a k8s node executing one etcd member pod reboots and comes back before node controller pod eviction triggers

##### Effect

The pod will go in failed state, the etcd cluster remains quorate. Etcd-operator will remove the member from the etcd cluster and add a new member to the cluster and creates a new pod with a new name.

#### Case B

##### Event

* a k8s node executing one etcd member pod becomes partitioned (network partitioning, goes down, etc...) and cannot talk to the api server, after some time the node controller will evict its pods.

##### Effect

The pod will go in an unknown/failed state, the etcd cluster remains quorate. Etcd-operator will remove the member from the etcd cluster and add a new member to the cluster and creates a new pod with a new name.

#### Case C

##### Event

One of these events happens:
* a majority of etcd processes exits (processes crashes, killed etc...) 
* k8s nodes scheduling a majority of etcd members pods reboots and come back before node controller pod eviction triggers

##### Effect

The pods will go in failed state, the etcd cluster becomes unquorate. Etcd-operator can only restore the cluster from a backup.

#### Case D

##### Event

* k8s nodes executing a majority of etcd members pods becomes partitioned (network partitioning, goes down, etc...) and cannot talk to the api server, after some time the node controller will evict their pods.

##### Effect

The pods will go in an unknown/failed state and eventually be removed from the API server, the etcd cluster becomes unquorate. Etcd-operator can only restore the cluster from a backup.


### Current failure handling

Currently etcd operator handles cases A and B but when a cluster becomes unquorate (cases C and D) the current option is to restore the cluster from a backup.

User may desire consistency over availability and prefer to wait for the cluster the return in a quorate state instead of restoring from a backup (that will contain old data).

This proposal aims to fix cases C and D permitting to the cluster the return in a quorate state without the need to restore from a backup.

## Proposed Changes

The design to be effective is split in two parts.

### Part 1

These changes will put the basis for initial persistent volume management

This first part won't fix case C and D.

- Add to the cluster spec PodPolicy an option to enable putting etcd data inside a Persistent Volume
- If podPolicy has persistent volumes enabled, when creating a new member also create a Persistent Volume Claim and use it as the volume source for etcd-data
- Handle etcd data persistent volume claims cleanup:
  - set PVC ownerRef to EtcdCluster object, which enables GC to clean up if cluster is deleted.
  - PVC lifecycle is bound to etcd member's lifecycle. PVC is added/removed when etcd member is added/removed (in the implementation we could choose to remove PVC only if there's not pod referencing them to avoid strange behaviors)

#### Pod name to Persistent Volume Claim name mapping

The pod and related PVC are mapped 1 to 1 so their names be easily generated and deducted. I.E. if the pod is called `etcd-0000` its related PVC name will be `etcd-0000-pvc`


### Part 2

Using persistent volumes to store etcd data will add additional failure cases:

- volume failure (failing to bind PV to pod).
- corrupted data (filesystem ok but missing/corrupted files) that will cause etcd to exit. Previously an etcd member with corrupted data will be deleted and a new member added, using a persistent volume we have to avoid reusing the same corrupted data volume.

This parts will fix cases C and D and the above corrupted data case.

It's a change to the reconcile logic to handle deleted pods from the k8s API server that had a persistent volume. To handle corrupted data failures, the reconcile logic change defined below will be used only if the cluster is unquorate. Instead, if the cluster is quorate, the current logic of deleting the member and adding a new member will be used.

- If the cluster is unquorate and a pod of an etcd cluster member is deleted from the API server and it had a persistent volume claim associated, instead of removing the etcd member from the etcd cluster and adding a new member just recreate it (keeping its pod name) binding the existing persistent volume claim. The pod recreation will happen only if the pod doesn't exists anymore in the API because we'll keep the same pod name. This also enable a MANDATORY property: at most once pod existence logic to avoid having in the API at the same time two pods with associated the same PVC.


### Future enhancements

Handle failures introduced by this design and not covered with the previous parts:

- volume failure (failing to bind PV to pod).



## Related issues

- [Explore Local PV](https://github.com/coreos/etcd-operator/issues/1201)
- [Persistent/Durable etcd cluster](https://github.com/coreos/etcd-operator/issues/1323)
- [Pod Safety, Consistency Guarantees, and Storage Implications](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/pod-safety.md)
