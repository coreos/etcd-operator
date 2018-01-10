# Release Workflow

**NOTE**: During release, don't merge any PR other than bumping the version.

Let's say we are releasing $VERSION (e.g. `v1.2.3`) .

## Create git tag

- Bump up version (e.g. 0.8.0 -> 0.8.1):
  ```bash
  ./hack/release/bump_version.sh 0.8.0 0.8.1
  ```

- Update CHANGELOG.md.

- Send a PR. After it's merged, cut a tag:

  ```bash
  git tag $VERSION
  git push ${upstream_remote} tags/$VERSION
  ```


## Push Image to Quay

- Login to quay.io using docker if haven't:

  ```bash
  docker login quay.io
  ```

Follow the prompts.

- git checkout tag (created above)

- Follow [developer_guide.md](./developer_guide.md) for build instructions.

- After build, push image:

```bash
$ IMAGE=quay.io/coreos/etcd-operator:$VERSION hack/build/docker_push
```

- Retag "latest":

```bash
$ docker tag quay.io/coreos/etcd-operator:$VERSION quay.io/coreos/etcd-operator:latest
$ docker push quay.io/coreos/etcd-operator:latest
```

## Github release

- Click "releases" in the top bar.

- Click "$VERSION" tag.

- Click "Edit Tag".

- Fill in title and release notes.

- If it's not stable release, click "This is a pre-release".

## Bump version

In version/version.go, bump version again:

```go
	// Version = "0.2.2+git" (without "v" prefix) 
	Version = "$VERSION+git"
```

Send another PR and merge it.
