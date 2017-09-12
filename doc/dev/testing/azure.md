This document explains how to run tests for etcd-operator functionality when using Azure Blob Storage (ABS) backups

## Integration Tests

ABS backend tests use the [azurite](https://github.com/arafato/azurite) tool to act as a lightweight clone for handling Azure Blob Storage client requests.

#### Run Azurite

See [azurite](https://github.com/arafato/azurite) for more options on running the tool.  Here we use the Docker image approach:

```
$ docker run -d -t -p 10000:10000 quay.io/vdice/azurite
```

#### Run tests

Simply set `RUN_INTEGRATION_TEST` to true and run the same unit tests as above.

```
$ RUN_INTEGRATION_TEST=true go test -v ./pkg/backup
```