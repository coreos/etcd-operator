Release Workflow
======

This docs describes the release process of kube-etcd-controller public docker image.

## Prerequisites

Make sure following tools are installed:
- glide
- docker

Make sure you have a quay.io account.

## Build image

Make sure your working directory is root of "kube-etcd-controller/".

Install dependency if none exists:
```
$ glide install
```
You should see "vendor/".

Build docker image
```
$ docker build --tag quay.io/coreos/kube-etcd-controller:${TAG} -f hack/build/controller/Dockerfile .
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
$ docker push quay.io/coreos/kube-etcd-controller:${TAG}
```
`${TAG}` is the same as above.
