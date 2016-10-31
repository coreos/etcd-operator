# etcd cluster lifecycle in etcd operator

Let's talk about the lifecycle of etcd cluster "A".

- Initially, "A" doesn't exist. Operator considers this cluster has 0 members.
  Any cluster with 0 members would be considered as non-existed.
- At some point of time, a user creates an object for "A".
  Operator would receive "ADDED" event and create this cluster.
  For the entire lifecycle, an etcd cluster could be created only once.
- Then user might update 0 or more times on the spec of "A".
  Operator would receive "MODIFIED" events and reconcile actual state gradually to desired state of given spec.
- Finally, a user deletes the object of "A". Operator will delete and recycle all resources of "A".
  For the entire lifecycle, an etcd cluster could be deleted only once.