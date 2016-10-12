FROM golang
ADD . /go/src/github.com/coreos/kube-etcd-controller
RUN go build -o kube-etcd-controller ./cmd/controller/main.go
RUN mv kube-etcd-controller $GOPATH/bin/
ENTRYPOINT ["kube-etcd-controller"]
