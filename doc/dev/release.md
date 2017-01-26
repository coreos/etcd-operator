# Release Workflow

This docs describes the release process of etcd-operator public quay image.

Login to quay.io using docker if haven't:

```
$ docker login quay.io
```
Follow the prompts.

Refer to [developer_guide.md](./developer_guide.md) on how to fetch dependency and build.

A quick look:
```
$ IMAGE=quay.io/coreos/etcd-operator:${TAG} hack/build/operator/build
```
`${TAG}` is the release tag. For example, "v0.0.1", "latest".

Create a corresponding release on github with release note.
