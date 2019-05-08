# golang:X-alpine can't be used since it does not support the race detector flag which assumes a glibc based system, whereas alpine linux uses musl libc
# https://github.com/golang/go/issues/14481
FROM golang:1.11.5 as builder

RUN wget -nv -O /bin/kubectl https://storage.googleapis.com/kubernetes-release/release/v1.12.6/bin/linux/amd64/kubectl

RUN chmod a+x /bin/kubectl

ADD ./ /go/src/github.com/coreos/etcd-operator

WORKDIR /go/src/github.com/coreos/etcd-operator

RUN go test ./test/e2e/ -c -o /bin/etcd-operator-e2e --race

FROM busybox:1.28.3-glibc

COPY --from=builder /bin/etcd-operator-e2e /bin
COPY --from=builder /bin/kubectl /bin
COPY --from=builder /go/src/github.com/coreos/etcd-operator/test/pod/simple/run-e2e /bin/run-e2e

CMD ["/bin/run-e2e"]
