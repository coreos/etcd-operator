#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Once https://github.com/kubernetes/kubernetes/pull/52186 is merged,
# we could use codegen.sh from code-generator repo.

DOCKER_REPO_ROOT="/go/src/github.com/coreos/etcd-operator"
IMAGE=${IMAGE:-"gcr.io/coreos-k8s-scale-testing/codegen"}

docker run --rm -ti \
  -v "$PWD":"$DOCKER_REPO_ROOT" \
  -w "$DOCKER_REPO_ROOT" \
  "$IMAGE" deepcopy-gen \
  --v 1 --logtostderr \
  --go-header-file "./hack/k8s/codegen/boilerplate.go.txt" \
  -i "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2" \
  -O zz_generated.deepcopy \
  --bounding-dirs "github.com/coreos/etcd-operator/pkg/apis"
