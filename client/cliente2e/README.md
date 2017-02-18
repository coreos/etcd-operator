# Client E2E Testing

Client e2e is testing pkg `experimentalclient/` .

To simplify operation, there is only one script to run:
```
./client/cliente2e/test
```

## Build image

Currently only etcd operator maintainers have access to test image.

Build and push image:

```bash
$ ./client/cliente2e/build
```

## Run e2e

```bash
$ go test ./client/cliente2e --kubeconfig ... --e2e-image ... --namespace ...
```
