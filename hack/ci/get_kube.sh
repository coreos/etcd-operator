#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

: ${DOWNLOAD_DIR:?"Need to set DOWNLOAD_DIR"}
KUBE_RELEASE_URL=${KUBE_RELEASE_URL:-kubernetes-release-dev/ci}

KUBE_VERSION=$(gsutil cat gs://${KUBE_RELEASE_URL}/latest.txt)
gsutil cp "gs://${KUBE_RELEASE_URL}/${KUBE_VERSION}/kubernetes.tar.gz" "${DOWNLOAD_DIR}/"

cd $DOWNLOAD_DIR
tar zxvf kubernetes.tar.gz 1>/dev/null

gsutil cp "gs://${KUBE_RELEASE_URL}/${KUBE_VERSION}/kubernetes-server-linux-amd64.tar.gz" "kubernetes/server/"
