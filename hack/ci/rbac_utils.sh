#!/usr/bin/env bash

: ${TEST_NAMESPACE:?"Need to set TEST_NAMESPACE"}
ROLE_NAME=etcd-operator-${TEST_NAMESPACE}
ROLE_BINDING_NAME=etcd-operator-${TEST_NAMESPACE}

function rbac_cleanup {
    kubectl delete clusterrolebinding ${ROLE_BINDING_NAME}
    kubectl delete clusterrole ${ROLE_NAME}
}

function rbac_setup() {
    # Create ClusterRole
    sed -e "s/<ROLE_NAME>/${ROLE_NAME}/g" example/rbac/cluster-role-template.yaml | kubectl create -f -

    # Create ClusterRoleBinding
    sed -e "s/<ROLE_NAME>/${ROLE_NAME}/g" \
      -e "s/<ROLE_BINDING_NAME>/${ROLE_BINDING_NAME}/g" \
      -e "s/<NAMESPACE>/${TEST_NAMESPACE}/g" \
      example/rbac/cluster-role-binding-template.yaml \
      | kubectl create -f -
}
