FROM tutum/curl:latest
ADD ./kube-etcd-controller /usr/local/bin/
ENTRYPOINT ["kube-etcd-controller"]
