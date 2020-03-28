# Developer guide

This document explains how to setup your dev environment. 

## Using Local Environment
### Fetch dependency

We use [dep](https://github.com/golang/dep) to manage dependency.
Install dependency if you haven't:

```
./hack/update_vendor.sh
```


### How to build
Requirement:
- Go 1.10+

Build in project root dir:

```
./hack/build/operator/build
./hack/build/backup-operator/build
./hack/build/restore-operator/build
```

## Using etcd-operator-builder
### Building the Builder
For convenience and easy of building, a Dockerfile has been provided to create an
etcd-operator-builder image.  This image contains a fully configured go envrionment
used for fetching dependencies and building the different etcd-operator binaries.

To build the etcd-operator-builder image, you first need to have docker installed.

Once installed, set the IMAGE environment variable to the desired name:
```
export IMAGE=etcd-operator-builder
```

Now, run the build script to generate the etcd-operator-builder:
```
./hack/build/e2e/builder/build
```

Once completed, the etcd-operator-builder:latest image should be available for use.


### Updating the Dependencies Using the Builder
Updating dependencies using this image can be performed by running the ci get_dep script:
```
./hack/ci/get_dep
```


### Building etcd-operator Using the Builder
Then build the project using the image:
```
./hack/build/build
```


## Creating the etcd-operator Docker Image
Finally, build the etcd-operator image:
```
export IMAGE=etcd-operator
docker build --tag "${IMAGE}" -f hack/build/Dockerfile . 1>/dev/null
```
