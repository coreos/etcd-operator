# Debug E2E Test Flake

E2E flakes will cost more and more effort and bring more and more trouble as time passes by.
It is better to solve them as early as possible.

Rules when a flake happened:

- Do not hide the failure.
  If test flake happened, don't just rerun the test.
  Comment or create an issue to track the problem.
- Figure out why the failure happened.
  The end goal should be either a code fix or some docs explaining the limitatons.

## How to debug

Figuring out the root cause of failures is usually easier and less time-consuming than it's assumed to.

Here is one process to help start the debugging process:

- Have a automated script to reproduce the test consistently. For example, a script to run locally:
  ```shell
  go test -c ./test/e2e
  for ((n=0;n<30;n++))
  do
  echo "${n} round..."
  ./e2e.test --kubeconfig=${HOME}/.kube/config \
      --operator-image=${IMAGE} \
      --test.run=${TEST_SELECTOR}
  sleep 15 # LE lock expiration
  done
  ```
  Note that there might be additional setup before running the test.
- Narrow down the root cause.
  In this phase, observability is the key. Add more logs, metrics.
- Once a bug is hunt down, follow up on the issue with detailed report.
