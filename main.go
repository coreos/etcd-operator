package main

import (
	"flag"

	"k8s.io/kubernetes/pkg/client/restclient"
)

var (
	masterHost  string
	tlsInsecure bool
	tlsConfig   restclient.TLSClientConfig
)

func init() {
	flag.StringVar(&masterHost, "master", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&tlsConfig.CertFile, "cert-file", "", " - NOT RECOMMENDED FOR PRODUCTION - Path to public TLS certificate file.")
	flag.StringVar(&tlsConfig.KeyFile, "key-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to private TLS certificate file.")
	flag.StringVar(&tlsConfig.CAFile, "ca-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to TLS CA file.")
	flag.BoolVar(&tlsInsecure, "tls-insecure", false, "- NOT RECOMMENDED FOR PRODUCTION - Don't verify API server's CA certificate.")
	flag.Parse()
}

func main() {
	c := &etcdClusterController{
		kclient:  mustCreateClient(masterHost),
		clusters: make(map[string]*Cluster),
	}
	c.Run()
}
