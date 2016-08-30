#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

pushd "./cmd/backup"
go build .
docker build --tag gcr.io/coreos-k8s-scale-testing/kubeetcdbackup:latest .
gcloud docker push gcr.io/coreos-k8s-scale-testing/kubeetcdbackup:latest
popd
