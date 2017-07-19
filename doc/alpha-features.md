# Alpha Features

Tracking document for alpha features of Kubernetes that etcd operator makes use of.

We track these alpha features as their behavior may change or be deprecated between Kubernetes versions.
Therefore clusters that use these features need to keep track of any potential changes in upstream releases.
See the upstream [api versioning documentation](https://github.com/kubernetes/community/blob/master/contributors/devel/api_changes.md#alpha-beta-and-stable-versions) for more information.


### TolerateUnreadyEndpointsAnnotation

Used by the etcd client and peer service object.

This alpha annotation will retain the endpoints even if the etcd pod isn't ready.
This feature is always enabled in endpoint controller in k8s even it is alpha.

References:
- https://github.com/coreos/etcd-operator/issues/622
- https://github.com/coreos/etcd-operator/issues/1257
