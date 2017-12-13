# Running Containarized Tests

The scripts at `test/pod` can be used to package and run the e2e tests inside a test-pod on a k8s cluster.

## Create the AWS secret

The e2e tests need access to an S3 bucket for testing. Create a secret containing the aws credentials and config files in the same namespace that the test-pod will run in. Consult the [backup-operator guide][setup-aws-secret] on how to do so.

## Build the test-pod image

The following should build and push the test-pod image to some specified repository:

```sh
TEST_IMAGE=gcr.io/coreos-k8s-scale-testing/etcd-operator-e2e test/pod/docker_push
```

## Generate the test-pod spec file

The [test-pod-tmpl.yaml](./test-pod-tmpl.yaml) can be used to define the test-pod spec with the necessary values for the following environment variables:

- `OPERATOR_IMAGE` is the etcd-operator image used for testing
- `TEST_S3_BUCKET` is the S3 bucket name used for testing
- `TEST_AWS_SECRET` is the secret name containing the aws credentials/config files.
- `E2E_TEST_SELECTOR` selects which e2e tests to run. Leave empty to run all tests.
- `UPGRADE_TEST_SELECTOR` selects which upgrade tests to run. Leave empty to run all tests.
- `PASSES` are the test passes to run (e2e, e2eslow, e2esh). See [hack/test](../../hack/test)
- `UPGRADE_FROM` is the starting image used in the upgrade tests
- `UPGRADE_TO` is the final image used in the upgrade tests

```sh
sed -e "s|<POD_NAME>|e2e-testing|g" \
    -e "s|<TEST_IMAGE>|gcr.io/coreos-k8s-scale-testing/etcd-operator-e2e|g" \
    -e "s|<PASSES>|e2eslow|g" \
    -e "s|<OPERATOR_IMAGE>|quay.io/coreos/etcd-operator:dev|g" \
    -e "s|<E2E_TEST_SELECTOR>||g" \
    -e "s|<TEST_S3_BUCKET>|my-bucket|g" \
    -e "s|<TEST_AWS_SECRET>|aws-secret|g" \
    -e "s|<UPGRADE_TEST_SELECTOR>||g" \
    -e "s|<UPGRADE_FROM>|quay.io/coreos/etcd-operator:latest|g" \
    -e "s|<UPGRADE_TO>|quay.io/coreos/etcd-operator:dev|g" \
    test/pod/test-pod-templ.yaml > test-pod-spec.yaml
```

## Run the test-pod

The `run-test-pod` script sets up RBAC for the namespace, runs the logcollector and then the test-pod. The script waits until the test-pod has run to completion.

```sh
  TEST_NAMESPACE=e2e \
    POD_NAME=testing-e2e \
    TEST_POD_SPEC=${PWD}/test-pod-spec.yaml \
    KUBECONFIG=/path/to/kubeconfig \
    test/pod/run-test-pod
```

The logfiles for all the pods in the test namespace will be saved at `${CWD}/_output/logs`.

[setup-aws-secret]:../../doc/user/walkthrough/backup-operator.md#setup-aws-secret
