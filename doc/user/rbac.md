# Operator RBAC setup

If RBAC is in place, users need to setup RBAC rules for etcd operator. This doc serves a tutorial for it.

## Quick setup

If you just want to play with etcd operator, there is a quick setup.

It assumes that your cluster has an admin role. For example, on [Tectonic](https://coreos.com/tectonic/),
there is a `admin` ClusterRole. We are using that here.

Modify or export env `$TEST_NAMESPACE` to a new namespace, then create it:

```bash
$ kubectl create ns $TEST_NAMESPACE
```

Then create cluster role binding:

```bash
$ cat <<EOF | kubectl create -f -
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: example-etcd-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: admin
subjects:
- kind: ServiceAccount
  name: default
  namespace: $TEST_NAMESPACE
EOF
```

Now you can start playing. One you are done, clean them up:

```bash
$ kubectl delete clusterrolebinding example-etcd-operator
$ kubectl delete ns $TEST_NAMESPACE
```

## Production setup

For production, we recommend users to limit access to only the resources operator needs, and create a specific role, service account for operator.

### Create ClusterRole

We will use ClusterRole instead of Role because etcd operator accesses non-namespaced resources, e.g. Third Party Resource.

Create the following ClusterRole

```bash
$ cat <<EOF | kubectl create -f -
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: etcd-operator
rules:
- apiGroups:
  - etcd.coreos.com
  resources:
  - clusters
  verbs:
  - "*"
- apiGroups:
  - extensions
  resources:
  - thirdpartyresources
  verbs:
  - create
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - create
- apiGroups: 
  - ""
  resources:
  - pods
  - services
  - endpoints
  - persistentvolumeclaims
  verbs:
  - "*"
- apiGroups:
  - extensions
  resources:
  - replicasets
  verbs:
  - "*"
EOF
```

If you need use s3 backup, add these to above input:

```
- apiGroups: 
  - ""
  resources: 
  - secrets
  - configmaps
  verbs:
  - get
```

### Create Service Account

Modify or export env `ETCD_OPERATOR_NS` to your current namespace, 
and create ServiceAccount for etcd operator:

```bash
$ cat <<EOF | kubectl create -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: etcd-operator
  namespace: $ETCD_OPERATOR_NS
EOF
```

### Create ClusterRoleBinding

Modify or export env `ETCD_OPERATOR_NS` to your current namespace, 
and create ClusterRoleBinding for etcd operator:

```bash
$ cat <<EOF | kubectl create -f -
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: etcd-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: etcd-operator
subjects:
- kind: ServiceAccount
  name: etcd-operator
  namespace: $ETCD_OPERATOR_NS
EOF
```

### Run deployment with service account

For etcd operator pod or deployment, fill the pod template with service account `etcd-operator` created above.

For example:

```yaml
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: etcd-operator
spec:
  template:
    spec:
      serviceAccountName: etcd-operator
      ...
```
