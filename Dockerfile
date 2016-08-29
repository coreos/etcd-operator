FROM tutum/curl:latest
ADD ./_output/bin/kube-etcd-controller /usr/local/bin/
ENTRYPOINT ["kube-etcd-controller"]
