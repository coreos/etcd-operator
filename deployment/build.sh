#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

go build .
docker build --tag gcr.io/coreos-k8s-scale-testing/kubeetcdctrl:latest .
gcloud docker push gcr.io/coreos-k8s-scale-testing/kubeetcdctrl:latest