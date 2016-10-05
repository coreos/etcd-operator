package main

import (
	"flag"
	"os"

	"github.com/coreos/kube-etcd-controller/pkg/controller"
)

var (
	cfg controller.Config
)

func init() {
	flag.StringVar(&cfg.PVProvisioner, "pv-provisioner", "kubernetes.io/gce-pd", "persistent volume provisioner type")
	flag.StringVar(&cfg.MasterHost, "master", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&cfg.TLSConfig.CertFile, "cert-file", "", " - NOT RECOMMENDED FOR PRODUCTION - Path to public TLS certificate file.")
	flag.StringVar(&cfg.TLSConfig.KeyFile, "key-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to private TLS certificate file.")
	flag.StringVar(&cfg.TLSConfig.CAFile, "ca-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to TLS CA file.")
	flag.BoolVar(&cfg.TLSInsecure, "tls-insecure", false, "- NOT RECOMMENDED FOR PRODUCTION - Don't verify API server's CA certificate.")
	flag.Parse()

	cfg.Namespace = os.Getenv("MY_POD_NAMESPACE")
	if len(cfg.Namespace) == 0 {
		cfg.Namespace = "default"
	}
}

func main() {
	c := controller.New(&cfg)
	c.Run()
}
