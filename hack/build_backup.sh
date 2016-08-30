#!/usr/bin/env bash

pushd "./cmd/backup"
go build .
docker build --tag gcr.io/coreos-k8s-scale-testing/kubeetcdbackup:latest .
gcloud docker push gcr.io/coreos-k8s-scale-testing/kubeetcdbackup:latest
popd
