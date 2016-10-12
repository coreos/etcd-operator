FROM golang
ADD . /go/src/github.com/coreos/kube-etcd-controller
RUN go install -o kube-etcd-controller ./cmd/controller/main.go
ENTRYPOINT ["kube-etcd-controller"]
