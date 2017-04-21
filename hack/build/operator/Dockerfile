FROM alpine:3.5

RUN apk add --no-cache ca-certificates

ADD _output/bin/etcd-operator /usr/local/bin
ADD _output/bin/etcd-backup /usr/local/bin

CMD ["etcd-operator"]
