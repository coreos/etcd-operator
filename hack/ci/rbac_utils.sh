#!/usr/bin/env bash

: ${TEST_NAMESPACE:?"Need to set TEST_NAMESPACE"}

function rbac_cleanup {
    kubectl delete clusterrolebinding "etcd-operator-${TEST_NAMESPACE}"
    kubectl delete clusterrole "etcd-operator-${TEST_NAMESPACE}"
}

function rbac_setup() {
    # Create ClusterRole
    cat <<EOF | kubectl create -f -
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: "etcd-operator-${TEST_NAMESPACE}"
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
  - "*"
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - "*"
- apiGroups:
  - ""
  resources:
  - pods
  - services
  - endpoints
  - persistentvolumeclaims
  - events
  verbs:
  - "*"
- apiGroups:
  - apps
  resources:
  - deployments
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
  name: "etcd-operator-${TEST_NAMESPACE}"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: "etcd-operator-${TEST_NAMESPACE}"
subjects:
- kind: ServiceAccount
  name: default
  namespace: $TEST_NAMESPACE
EOF
}
