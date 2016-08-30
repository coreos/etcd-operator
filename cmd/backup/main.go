package main

import (
	"flag"

	"github.com/coreos/kube-etcd-controller/pkg/backup"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
)

var (
	masterHost  string
	clusterName string
)

func init() {
	flag.StringVar(&masterHost, "master", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&clusterName, "etcd-cluster", "", "")
	// TODO: parse policy
	flag.Parse()
}

func main() {
	if len(clusterName) == 0 {
		panic("clusterName not set")
	}
	kclient := k8sutil.MustCreateClient(masterHost, false, nil)
	backup.New(kclient, clusterName, backup.Policy{}).Run()
}
