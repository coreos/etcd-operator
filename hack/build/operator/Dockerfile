FROM golang

ADD . /go/src/github.com/coreos/etcd-operator

ARG ARG_LDFLAGS

RUN cd /go/src/github.com/coreos/etcd-operator && \
	go build -ldflags "${ARG_LDFLAGS:-}" -o etcd-operator ./cmd/operator/main.go && \
	mv etcd-operator /usr/local/bin && \
	go build -ldflags "${ARG_LDFLAGS:-}" -o etcd-backup ./cmd/backup/main.go && \
	mv etcd-backup /usr/local/bin && \
	rm -rf /go/*

CMD ["/bin/sh", "-c", "/usr/local/bin/etcd-operator"]
