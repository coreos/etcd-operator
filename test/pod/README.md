# Running Containarized Tests

The scripts at `test/pod` can be used to package and run the e2e tests inside a [test-pod](./test-pod.yaml) on a k8s cluster.

## Create the AWS secret

The e2e tests need access to an S3 bucket for testing. Create a secret containing the aws credentials and config files in the same namespace that the test-pod will run in. Consult the [backup-operator guide][setup-aws-secret] on how to do so.


## Build the test-pod image

The following should build and push the test-pod image to some specified repository:

```sh
TEST_IMAGE=gcr.io/coreos-k8s-scale-testing/etcd-operator-tests test/pod/build
```

## Run the test-pod
The `run-test-pod` script sets up RBAC for the namespace, and necessary environment variables for the test-pod before running it:

- `TEST_NAMESPACE` is the namespace where the test-pod and e2e tests will run.
- `OPERATOR_IMAGE` is the etcd-operator image used for testing
- `TEST_S3_BUCKET` is the S3 bucket name used for testing
- `TEST_AWS_SECRET` is the secret name containing the aws credentials/config files.

```sh
TEST_IMAGE=gcr.io/coreos-k8s-scale-testing/etcd-operator-tests \
TEST_NAMESPACE=e2e \
OPERATOR_IMAGE=quay.io/coreos/etcd-operator:dev \
TEST_S3_BUCKET=my-bucket \
TEST_AWS_SECRET=aws-secret \
test/pod/run-test-pod
```

[setup-aws-secret]:../../doc/user/walkthrough/backup-operator.md#setup-aws-secret