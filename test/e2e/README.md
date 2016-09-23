# E2E Testing

End-to-end (e2e) testing is automated testing for real user scenarios.

## Build and run test

Prerequisites:
- a running k8s cluster and kube config. We will need to pass kube config as arguments.
- KUBERNETES_KUBECONFIG_PATH env var is required, e.g. `KUBERNETES_KUBECONFIG_PATH=$HOME/.kube/config`

As first step, we need to setup environment for testing:
```
$ ./hack/e2e setup
```
This will do all preparation work such as creating etcd controller.

e2e tests are written as go test. All go test techniques applies, e.g. picking what to run, timeout length.
Let's say I want to run all tests in "test/e2e/":
```
$ go test -v ./test/e2e/
```

Finally, we need to tear down the things we setup before:
```
$ ./hack/e2e teardown
```
