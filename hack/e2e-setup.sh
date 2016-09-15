#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

mkdir -p _output/bin || true

go build -o _output/bin/setup ./test/e2e/framework/setup/main.go
_output/bin/setup --kubeconfig ${KUBERNETES_KUBECONFIG_PATH}
