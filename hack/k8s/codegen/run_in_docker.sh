#!/usr/bin/env bash

# This script is designed to run inside docker.

set -o errexit
set -o nounset
set -o pipefail

source ./$(dirname "$0")/codegen.sh

codegen::generate-groups all \
  github.com/coreos/etcd-operator/pkg/generated \
  github.com/coreos/etcd-operator/pkg/apis \
  etcd:v1beta2 \
  --go-header-file "./hack/k8s/codegen/boilerplate.go.txt"

echo "success..."
