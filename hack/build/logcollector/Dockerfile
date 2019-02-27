# Build step: docker build --tag gcr.io/coreos-k8s-scale-testing/logcollector -f hack/build/logcollector/Dockerfile .

FROM golang:1.11.5

ADD ./ /go/src/github.com/coreos/etcd-operator

WORKDIR /go/src/github.com/coreos/etcd-operator

RUN rm -rf _output _test .git .gitignore

RUN go build -o /usr/local/bin/logcollector test/logcollector/main.go
