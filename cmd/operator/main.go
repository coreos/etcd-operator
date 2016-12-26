package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/GregoryIan/operator/pkg/controller"
	"github.com/GregoryIan/operator/pkg/util"
	"github.com/GregoryIan/operator/version"

	"github.com/juju/errors"
	"github.com/ngaut/log"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/leaderelection"
	"k8s.io/kubernetes/pkg/client/record"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

var (
	masterHost string

	tlsInsecure bool
	certFile    string
	keyFile     string
	caFile      string

	namespace string

	printVersion bool
)

var (
	leaseDuration = 15 * time.Second
	renewDuration = 5 * time.Second
	retryPeriod   = 3 * time.Second
)

func init() {
	flag.StringVar(&masterHost, "master", "", "API Server addr, e.g. 'http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&certFile, "cert-file", "", "Path to public TLS certificate file.")
	flag.StringVar(&keyFile, "key-file", "", "Path to private TLS certificate file.")
	flag.StringVar(&caFile, "ca-file", "", "Path to TLS CA file.")
	flag.BoolVar(&tlsInsecure, "tls-insecure", false, "Don't verify API server's CA certificate.")
	flag.BoolVar(&printVersion, "version", false, "Show version and quit")
	flag.Parse()

	namespace = os.Getenv("MY_POD_NAMESPACE")
	if len(namespace) == 0 {
		namespace = "default"
	}
}

func main() {
	if printVersion {
		fmt.Println("tidb-operator", version.Version)
		os.Exit(0)
	}

	id, err := os.Hostname()
	if err != nil {
		log.Fatalf("failed to get hostname: %v", err)
	}
	// leaderelection for multiple tidb operators
	leaderelection.RunOrDie(leaderelection.LeaderElectionConfig{
		EndpointsMeta: api.ObjectMeta{
			Namespace: namespace,
			Name:      "tidb-operator",
		},
		Client: util.MustCreateClient(masterHost, tlsInsecure, &restclient.TLSClientConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
			CAFile:   caFile,
		}),
		EventRecorder: &record.FakeRecorder{},
		Identity:      id,
		LeaseDuration: leaseDuration,
		RenewDeadline: renewDuration,
		RetryPeriod:   retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				log.Fatalf("leader election lost")
			},
		},
	})
	panic("unreachable")
}

func run(stop <-chan struct{}) {
	for {
		kubeCli := createKubeClient()
		if len(masterHost) == 0 {
			log.Info("use in cluster client from k8s")
			masterHost = util.MustGetInClusterMasterHost()
		}

		c := controller.New(masterHost, namespace, kubeCli)
		err := c.Run()
		switch err {
		case controller.ErrVersionOutdated:
		default:
			log.Fatalf("controller Run() ended with failure: %v", errors.Trace(err))
		}
	}
}

func createKubeClient() *unversioned.Client {
	tlsConfig := restclient.TLSClientConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
		CAFile:   caFile,
	}
	return util.MustCreateClient(masterHost, tlsInsecure, &tlsConfig)
}
