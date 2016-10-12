package main

import (
	"flag"
	"os"
	"time"

	"github.com/coreos/kube-etcd-controller/pkg/analytics"
	"github.com/coreos/kube-etcd-controller/pkg/controller"
)

var (
	cfg              controller.Config
	analyticsEnabled bool
)

func init() {
	flag.StringVar(&cfg.PVProvisioner, "pv-provisioner", "kubernetes.io/gce-pd", "persistent volume provisioner type")
	flag.BoolVar(&analyticsEnabled, "analytics", true, "Send analytical event (Cluster Created/Deleted etc.) to Google Analytics")
	flag.StringVar(&cfg.EtcdTLSConfig.Dir, "etcd-tls-dir", "/etc/kube-etcd-controller/tls/", "directory to store etcd tls files. in most cases, this path will be mounted in a secret.")
	flag.StringVar(&cfg.EtcdTLSConfig.CommonName, "controller-ca-cn", "kube-etcd-controller", "common name for generated controller ca")
	flag.StringVar(&cfg.EtcdTLSConfig.Organization, "controller-ca-org", "coreos", "organization for generated controller ca")
	flag.DurationVar(&cfg.EtcdTLSConfig.Duration, "controller-ca-dur", 90*24*time.Hour, "duration (time until expiry) for generated controller ca")
	flag.StringVar(&cfg.MasterHost, "master", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&cfg.TLSConfig.CertFile, "cert-file", "", " - NOT RECOMMENDED FOR PRODUCTION - Path to public TLS certificate file.")
	flag.StringVar(&cfg.TLSConfig.KeyFile, "key-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to private TLS certificate file.")
	flag.StringVar(&cfg.TLSConfig.CAFile, "ca-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to TLS CA file.")
	flag.BoolVar(&cfg.TLSInsecure, "tls-insecure", false, "- NOT RECOMMENDED FOR PRODUCTION - Don't verify API server's CA certificate.")
	//TODO(chom): figure out external PKI story, integrate with some sort of remote signing agent, FLIP THIS TO FALSE
	flag.BoolVar(&cfg.EtcdTLSConfig.SelfSignedControllerCA, "self-signed-controller-ca", true, "- NOT RECOMMENDED FOR PRODUCTION - Generate self-signed controller CA key/cert on startup. Don't submit CSR to remote signing agent.")
	flag.Parse()

	cfg.Namespace = os.Getenv("MY_POD_NAMESPACE")
	if len(cfg.Namespace) == 0 {
		cfg.Namespace = "default"
	}
	if err := os.MkdirAll(cfg.EtcdTLSConfig.Dir, 0600); err != nil {
		panic(err)
	}
}

func main() {
	if analyticsEnabled {
		analytics.Enable()
	}

	c := controller.New(&cfg)
	analytics.ControllerStarted()
	c.Run()
}
