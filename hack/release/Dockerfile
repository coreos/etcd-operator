FROM golang

ADD . /go/src/github.com/coreos/kube-etcd-controller

RUN cd /go/src/github.com/coreos/kube-etcd-controller && \
	go build -o kube-etcd-controller ./cmd/controller/main.go && \
	mv kube-etcd-controller /usr/local/bin && \
	rm -rf /go/*

ENTRYPOINT ["kube-etcd-controller"]
