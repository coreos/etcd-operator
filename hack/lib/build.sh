#!/usr/bin/env bash

function go_build {
	echo "building "${1}"..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $GO_BUILD_FLAGS -o ${bin_dir}/etcd-${1} -installsuffix cgo -ldflags "$go_ldflags" ./cmd/${1}/
}
