## Client service
For every etcd cluster created the etcd-operator will create an etcd client service in the same namespace with the name `<cluster-name>-client` .
```
$ kubectl create -f example/example-etcd-cluster.yaml
$ kubectl get services
NAME                          CLUSTER-IP     EXTERNAL-IP   PORT(S)    AGE
example-etcd-cluster          None           <none>        2380/TCP   1m
example-etcd-cluster-client   10.0.222.115   <none>        2379/TCP   1m
```

The client service is of type `ClusterIP` and only accessible from within the Kubernetes cluster's network.

So for instance we can access the service from a pod in our cluster:
```
$ kubectl run --rm -i --tty fun --image quay.io/coreos/etcd --restart=Never -- /bin/sh
/ # ETCDCTL_API=3 etcdctl --endpoints http://example-etcd-cluster-client:2379 put foo bar
OK
(ctrl-D to exit)
```
If accessing this service from a different namespace than that of the etcd cluster, use the FQDN `http://<cluster-name>-client.<cluster-namespace>.svc.cluster.local:2379` .

### Accessing the service from outside the cluster

In order to access the client API of the etcd cluster from outside the Kubernetes cluster, we can expose a new client service of type `LoadBalancer`. If using a cloud provider like GKE/GCE or AWS, setting the type to `LoadBalancer` will automatically create the load balancer with a publicly accessible IP.

The spec for this service will use the label selector `etcd_cluster: <cluster-name>` to load balance the client requests over the etcd pods in our cluster.

So for our example cluster above we can create a service like so:
```
$ cat example-etcd-client-service-lb.yaml
apiVersion: v1
kind: Service
metadata:
  name: example-etcd-client-service-lb
  namespace: default
spec:
  ports:
  - name: client
    port: 2379
    protocol: TCP
    targetPort: 2379
  selector:
    etcd_cluster: example-etcd-cluster
  type: LoadBalancer

$ kubectl create -f example-etcd-client-service-lb.yaml
```

Wait until the load balancer is created and the service is assigned an `EXTERNAL-IP`:
```
$ kubectl get services
NAME                             CLUSTER-IP     EXTERNAL-IP     PORT(S)          AGE
example-etcd-cluster             None           <none>          2380/TCP         5m
example-etcd-cluster-client      10.0.222.115   <none>          2379/TCP         5m
example-etcd-cluster-client-lb   10.0.176.134   35.184.74.127   2379:32478/TCP   1m
```

The etcd client API should now be accessible from outside the kubernetes cluster:
```
$ ETCDCTL_API=3 etcdctl --endpoints http://35.184.74.127:2379 get foo
foo
bar
```