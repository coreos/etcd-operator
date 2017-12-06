function go_build {
	echo "building "${1}"..."
	# We’re disabling cgo which gives us a static binary.
	# This is needed for building minimal container based on alpine image.
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $GO_BUILD_FLAGS -o ${bin_dir}/etcd-${1} -installsuffix cgo -ldflags "$go_ldflags" ./cmd/${1}/
}
