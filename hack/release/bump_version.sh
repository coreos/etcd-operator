#!/usr/bin/env bash

# Usage:
#   ./hack/release/bump_version.sh 0.8.0 0.8.1

oldv=$1
newv=$2

echo "old version: ${oldv}, new version: ${newv}"

sed -i.bak -e "s/${oldv}+git/${newv}/g" version/version.go
sed -i.bak -e "s/${oldv}/${newv}/g" example/deployment.yaml
sed -i.bak -e "s/${oldv}/${newv}/g" example/etcd-backup-operator/deployment.yaml
sed -i.bak -e "s/${oldv}/${newv}/g" example/etcd-restore-operator/deployment.yaml

rm version/version.go.bak
rm example/deployment.yaml.bak
rm example/etcd-backup-operator/deployment.yaml.bak
rm example/etcd-restore-operator/deployment.yaml.bak
