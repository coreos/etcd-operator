FROM alpine

ADD _output/bin/etcd-operator /usr/local/bin
ADD _output/bin/etcd-backup /usr/local/bin

CMD ["/bin/sh", "-c", "/usr/local/bin/etcd-operator"]
