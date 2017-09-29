FROM alpine:3.6
RUN apk add --no-cache ca-certificates

ADD _output/bin/etcd-backup-operator /usr/local/bin/etcd-backup-operator

CMD ["etcd-backup-operator"]

