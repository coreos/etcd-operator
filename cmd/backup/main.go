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
	"fmt"
	"os"

	"github.com/coreos/kube-etcd-controller/pkg/backup"
	"github.com/coreos/kube-etcd-controller/pkg/spec"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	"github.com/coreos/kube-etcd-controller/version"
)

var (
	masterHost  string
	clusterName string
	listenAddr  string
	namespace   string

	printVersion bool
)

func init() {
	flag.StringVar(&masterHost, "master", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&clusterName, "etcd-cluster", "", "")
	flag.StringVar(&listenAddr, "listen", "0.0.0.0:19999", "")
	flag.BoolVar(&printVersion, "version", false, "Show version and quit")

	// TODO: parse policy
	flag.Parse()

	namespace = os.Getenv("MY_POD_NAMESPACE")
	if len(namespace) == 0 {
		namespace = "default"
	}
}

func main() {
	if printVersion {
		fmt.Println("kube-etcd-backup", version.Version)
		os.Exit(0)
	}

	if len(clusterName) == 0 {
		panic("clusterName not set")
	}
	kclient := k8sutil.MustCreateClient(masterHost, false, nil)
	backup.New(kclient, clusterName, namespace, spec.BackupPolicy{}, listenAddr).Run()
}
