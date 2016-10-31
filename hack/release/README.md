Release Workflow
======

This docs describes the release process of etcd-operator public docker image.

## Prerequisites

Make sure following tools are installed:
- glide
- docker

Make sure you have a quay.io account.

## Build image

Make sure your working directory is root of "etcd-operator/".

Install dependency if none exists:
```
$ glide install
```
You should see "vendor/".

Build docker image
```
$ docker build --tag quay.io/coreos/etcd-operator:${TAG} -f hack/build/operator/Dockerfile .
```
`${TAG}` is the release tag. For example, "v0.0.1", "latest".
We also need to create a corresponding release on github with release note.

## Push to quay.io

Login to quay.io using docker if not done before:
```
$ docker login quay.io
```
Follow the prompts.

Push docker image to quay.io:
```
$ docker push quay.io/coreos/etcd-operator:${TAG}
```
`${TAG}` is the same as above.
