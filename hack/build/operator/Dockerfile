FROM golang

ADD . /go/src/github.com/coreos/etcd-operator

RUN cd /go/src/github.com/coreos/etcd-operator && \
	go build -o etcd-operator ./cmd/operator/main.go && \
	mv etcd-operator /usr/local/bin && \
	rm -rf /go/*

CMD ["/bin/sh", "-c", "/usr/local/bin/etcd-operator"]
