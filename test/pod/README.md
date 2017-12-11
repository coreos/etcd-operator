# Running Containarized Tests

The scripts at `test/pod` can be used to package and run the e2e tests inside a [test-pod](./test-pod.yaml) on a k8s cluster.

## Create the AWS secret

The e2e tests need access to an S3 bucket for testing. Create a secret containing the aws credentials and config files in the same namespace that the test-pod will run in. Consult the [backup-operator guide][setup-aws-secret] on how to do so.

## Build the test-pod image

The following should build and push the test-pod image to some specified repository:

```sh
TEST_IMAGE=gcr.io/coreos-k8s-scale-testing/etcd-operator-e2e test/pod/build
```

## Run the test-pod

We first render test-pod yaml template. It has the following variables:

- `TEST_NAMESPACE` is the namespace where the test-pod and e2e tests will run.
- `TEST_IMAGE` is the image that will run the e2e test.
- `OPERATOR_IMAGE` is the etcd-operator image used for testing.
- `PASSES` is some test passes defined in `hack/test`.
- `E2E_TEST_SELECTOR` selects the tests to run. Blank ("") to run all tests. To run a particular test set to test name e.g: CreateCluster.
- `TEST_S3_BUCKET` is the S3 bucket name used for testing
- `TEST_AWS_SECRET` is the secret name containing the aws credentials/config files.

Then we run `run-test-pod` script to set up RBAC for the namespace, and run test pod.

```sh
export POD_NAME="e2e-testing"

sed -e "s|<POD_NAME>|${POD_NAME}|g" \
    -e 's|<TEST_IMAGE>|gcr.io/coreos-k8s-scale-testing/etcd-operator-e2e|g' \
    -e 's|<OPERATOR_IMAGE>|quay.io/coreos/etcd-operator:dev|g' \
    -e 's|<PASSES>|"e2e e2eslow"|g' \
    -e 's|<E2E_TEST_SELECTOR>||g' \
    -e 's|<TEST_S3_BUCKET>|my-bucket|g' \
    -e 's|<TEST_AWS_SECRET>|aws-secret|g' \
    test/pod/test-pod.yaml > _output/test-pod.yaml

TEST_NAMESPACE=e2e test/pod/run-test-pod _output/test-pod.yaml
```

[setup-aws-secret]:../../doc/user/walkthrough/backup-operator.md#setup-aws-secret
