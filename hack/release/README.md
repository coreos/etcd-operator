Release Workflow
======

This docs describes the release process of etcd-operator public docker image.

## Prerequisites

Supported build arch
- linux/amd64

Make sure the following tools are installed:
- glide
- docker
- go 1.7

Have a quay.io account.

## Build image

Working directory is root of "etcd-operator/".

Install dependency if none exists:
```
$ glide install
```
You should see "vendor/".

Login to quay.io using docker if haven't:
```
$ docker login quay.io
```
Follow the prompts.

Use build script to build binaries, docker images, and push to quay registry:
```
$ IMAGE=quay.io/coreos/etcd-operator:${TAG} hack/build/operator/build
```
`${TAG}` is the release tag. For example, "v0.0.1", "latest".

We also need to create a corresponding release on github with release note.
