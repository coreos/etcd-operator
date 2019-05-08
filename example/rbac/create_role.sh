#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

ETCD_OPERATOR_ROOT=$(dirname "${BASH_SOURCE}")/../..

print_usage() {
  echo "$(basename "$0") - Create Kubernetes RBAC role and role bindings for etcd-operator
Usage: $(basename "$0") [options...]
Options:
  --role-name=STRING         Name of ClusterRole to create
                               (default=\"etcd-operator\", environment variable: ROLE_NAME)
  --role-binding-name=STRING Name of ClusterRoleBinding to create
                               (default=\"etcd-operator\", environment variable: ROLE_BINDING_NAME)
  --namespace=STRING         namespace to create role and role binding in. Must already exist.
                               (default=\"default\", environment variable: NAMESPACE)
" >&2
}

ROLE_NAME="${ROLE_NAME:-etcd-operator}"
ROLE_BINDING_NAME="${ROLE_BINDING_NAME:-etcd-operator}"
NAMESPACE="${NAMESPACE:-default}"

for i in "$@"
do
case $i in
    --role-name=*)
    ROLE_NAME="${i#*=}"
    ;;
    --role-binding-name=*)
    ROLE_BINDING_NAME="${i#*=}"
    ;;
    --namespace=*)
    NAMESPACE="${i#*=}"
    ;;
    -h|--help)
      print_usage
      exit 0
    ;;
    *)
      print_usage
      exit 1
    ;;
esac
done

echo "Creating role with ROLE_NAME=${ROLE_NAME}, NAMESPACE=${NAMESPACE}"
sed -e "s/<ROLE_NAME>/${ROLE_NAME}/g" \
  -e "s/<NAMESPACE>/${NAMESPACE}/g" \
  "${ETCD_OPERATOR_ROOT}/example/rbac/cluster-role-template.yaml" | \
  kubectl create -f -

echo "Creating role binding with ROLE_NAME=${ROLE_NAME}, ROLE_BINDING_NAME=${ROLE_BINDING_NAME}, NAMESPACE=${NAMESPACE}"
sed -e "s/<ROLE_NAME>/${ROLE_NAME}/g" \
  -e "s/<ROLE_BINDING_NAME>/${ROLE_BINDING_NAME}/g" \
  -e "s/<NAMESPACE>/${NAMESPACE}/g" \
  "${ETCD_OPERATOR_ROOT}/example/rbac/cluster-role-binding-template.yaml" | \
  kubectl create -f -
