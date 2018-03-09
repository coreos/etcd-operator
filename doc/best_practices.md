# Best practices

## Large-scale deployment

To run etcd cluster at large-scale, it is important to assign etcd pods to nodes with desired resources, such as SSD, high performance network.

### Assign to nodes with desired resources

Kubernetes nodes can be attached with labels. Users can [assign pods to nodes with given labels](http://kubernetes.io/docs/user-guide/node-selection/). Similarly for the etcd-operator, users can specify `Node Selector` in the pod policy to select nodes for etcd pods. For example, users can label a set of nodes with SSD with label `"disk"="SSD"`. To assign etcd pods to these node, users can specify `"disk"="SSD"` node selector in the cluster spec.

### Assign to dedicated node

Even with container isolation, not all resources are isolated. Thus, performance interference can still affect etcd clusters' performance unexpectedly. We recommend dedicating nodes for each etcd pod to achieve predictable performance.

Kubernetes node can be [tainted](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/scheduling/taint-toleration-dedicated.md) with keys. Together with node selector feature, users can create nodes that are dedicated for only running etcd clusters. This is the **suggested way** to run high performance large scale etcd clusters.

Use kubectl to taint the node with kubectl taint nodes etcd dedicated. Then only etcd pods, which tolerate this taint will be assigned to the nodes.
