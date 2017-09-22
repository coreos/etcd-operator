# Status Conditions and Events

To make it easier for users to understand and debug the etcd-operator, the actions of the operator and the state of the cluster are communicated to the user in the standard Kubernetes convention. 

In Kubernetes users will normally view more information about an object via `kubectl describe` which displays, among other things, the [Events](https://kubernetes.io/docs/api-reference/v1.7/#event-v1-core) and [Conditions](https://kubernetes.io/docs/api-reference/v1.7/#podcondition-v1-core) associated with the resource.

Similarly the etcd-operator exposes the Events and Conditions for each EtcdCluster Custom Resource.

## Events
The following types of Events and their specific instances are common in the lifecycle of an EtcdCluster:

- A new member is added
- A member is removed
- A member is upgraded
- Replace a dead member

## Conditions

The etcd cluster Condition and its statuses are defined as:

- Available
  - True: Majority members up
  - False: Reason for not being available (majority down only)
- Recovering
  - True: Reason for recovery (all members down, or majority down)
  - False: Reason for recovery failure (e.g no backup found)
  - Not present
- Scaling
  - True: Scaling from current members size X to spec.size Y
  - False: Reason for failure (e.g no more nodes to place member due to anti-affinity)
  - Not present
- Upgrading
  - True: Upgrading from version X to Y
  - False: Reason for failure
  - Not present
