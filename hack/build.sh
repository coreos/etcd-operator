#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

 if ! which go > /dev/null; then
 	echo "go needs to be installed"
 	exit 1
 fi

 if ! which docker > /dev/null; then
 	echo "docker needs to be installed"
 	exit 1
 fi

go build .
docker build --tag gcr.io/coreos-k8s-scale-testing/kubeetcdctrl:latest .
gcloud docker push gcr.io/coreos-k8s-scale-testing/kubeetcdctrl:latest
