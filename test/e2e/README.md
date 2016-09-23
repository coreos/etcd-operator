# E2E Testing

End-to-end (e2e) testing is automated testing for real user scenarios.

## Build and run test

Prerequisites:
- a running k8s cluster and kube config. We will need to pass kube config as arguments.
- Have kubeconfig file ready.
- Have etcd controller image ready.

e2e tests are written as go test. All go test techniques applies, e.g. picking what to run, timeout length.
Let's say I want to run all tests in "test/e2e/":
```
$ go test -v ./test/e2e/ --kubeconfig "$HOME/.kube/config" --controller-image gcr.io/coreos-k8s-scale-testing/kube-etcd-controller:latest
```
