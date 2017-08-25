This document explains how to run tests for etcd-operator functionality

## Unit Tests

To run unit tests, supply the path to all directories containing tests intended to be run.  Here we wish to test `pkg/backup`:

```
$ go test -v ./pkg/backup
```

## Integration Tests

Integration-level tests can be run by passing the appropriate flag, assumming the required initial setup has been run.

### ABS integration tests

ABS backend tests use the [azurite](https://github.com/arafato/azurite) tool to act as a lightweight clone for handling Azure Blob Storage client requests.

#### Run Azurite

See [azurite](https://github.com/arafato/azurite) for more options on running the tool.  Here we use the Docker image approach:

```
$ docker run -d -t -p 10000:10000 quay.io/vdice/azurite
```

#### Run tests

```
$ RUN_ABS_INTEGRATION_TEST=true go test -v ./pkg/backup
```

## E2E Tests

See the [E2E Tests](../../test/e2e/README.md) doc for further info.