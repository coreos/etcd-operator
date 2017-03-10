FROM alpine:3.5

RUN adduser -D etcd-operator
RUN apk add --no-cache ca-certificates

ADD _output/bin/etcd-operator /usr/local/bin
ADD _output/bin/etcd-backup /usr/local/bin

USER etcd-operator

CMD ["/bin/sh", "-c", "/usr/local/bin/etcd-operator"]
