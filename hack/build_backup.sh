#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

pushd "./cmd/backup"

CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o backup .

docker build --tag gcr.io/coreos-k8s-scale-testing/kubeetcdbackup:latest .
gcloud docker push gcr.io/coreos-k8s-scale-testing/kubeetcdbackup:latest
popd
