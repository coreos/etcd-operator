# Developer guide

This document explains how to setup your dev environment. 

## Fetch dependency

We use [dep](https://github.com/golang/dep) to manage dependency.
Install dependency if you haven't:

```
./hack/update_vendor.sh
```

## How to build

Requirement:
- Go 1.10+

Build in project root dir:

```
./hack/build/operator/build
./hack/build/backup-operator/build
./hack/build/restore-operator/build
```
