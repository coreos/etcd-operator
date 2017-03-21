#!/usr/bin/env bash

: ${TEST_NAMESPACE:?"Need to set TEST_NAMESPACE"}

function rbac_cleanup {
	kubectl delete clusterrolebinding etcd-operator
	kubectl delete clusterrole etcd-operator
}

function rbac_setup() {
    # Create ClusterRole
    cat <<EOF | kubectl create -f -
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
- apiGroups:
  - ""
  resources:
  - secrets
  - configmaps
  verbs:
  - get
EOF

    # Create ClusterRoleBinding in namespace
    cat <<EOF | kubectl create -f -
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
  name: default
  namespace: $TEST_NAMESPACE
EOF
}
