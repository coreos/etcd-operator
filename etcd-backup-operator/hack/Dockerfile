FROM alpine
RUN apk add --no-cache ca-certificates
ADD _output/bin/ /usr/local/bin

CMD ["etcd-backup-operator"]

