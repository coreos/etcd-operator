// Copyright 2016 The kube-etcd-controller Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/pkg/analytics"
	"github.com/coreos/kube-etcd-controller/pkg/controller"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	"k8s.io/kubernetes/pkg/client/restclient"
)

var (
	analyticsEnabled bool
	pvProvisioner    string
	masterHost       string
	tlsInsecure      bool
	certFile         string
	keyFile          string
	caFile           string
	namespace        string
)

func init() {
	flag.BoolVar(&analyticsEnabled, "analytics", true, "Send analytical event (Cluster Created/Deleted etc.) to Google Analytics")

	flag.StringVar(&pvProvisioner, "pv-provisioner", "kubernetes.io/gce-pd", "persistent volume provisioner type")
	flag.StringVar(&masterHost, "master", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&certFile, "cert-file", "", " - NOT RECOMMENDED FOR PRODUCTION - Path to public TLS certificate file.")
	flag.StringVar(&keyFile, "key-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to private TLS certificate file.")
	flag.StringVar(&caFile, "ca-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to TLS CA file.")
	flag.BoolVar(&tlsInsecure, "tls-insecure", false, "- NOT RECOMMENDED FOR PRODUCTION - Don't verify API server's CA certificate.")
	flag.Parse()

	namespace = os.Getenv("MY_POD_NAMESPACE")
	if len(namespace) == 0 {
		namespace = "default"
	}
}

func main() {
	if analyticsEnabled {
		analytics.Enable()
	}

	analytics.ControllerStarted()

	cfg := newControllerConfig()
	c := controller.New(cfg)
	c.Run()
}

func newControllerConfig() controller.Config {
	tlsConfig := restclient.TLSClientConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
		CAFile:   caFile,
	}
	kubecli := k8sutil.MustCreateClient(masterHost, tlsInsecure, &tlsConfig)
	cfg := controller.Config{
		MasterHost:    masterHost,
		PVProvisioner: pvProvisioner,
		Namespace:     namespace,
		KubeCli:       kubecli,
	}
	if len(cfg.MasterHost) == 0 {
		logrus.Info("use in cluster client from k8s library")
		cfg.MasterHost = k8sutil.MustGetInClusterMasterHost()
	}
	return cfg
}
