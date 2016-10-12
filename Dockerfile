FROM golang

ADD . /go/src/github.com/coreos/kube-etcd-controller

RUN go get github.com/Masterminds/glide
RUN cd /go/src/github.com/coreos/kube-etcd-controller && \
	glide install && \
	go build -o kube-etcd-controller ./cmd/controller/main.go && \
	mv kube-etcd-controller $GOPATH/bin/

ENTRYPOINT ["kube-etcd-controller"]
