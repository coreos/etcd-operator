// Copyright 2016 The etcd-operator Authors
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
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup"
	"github.com/coreos/etcd-operator/pkg/backup/env"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/version"

	"github.com/sirupsen/logrus"
)

var (
	masterHost      string
	clusterName     string
	listenAddr      string
	namespace       string
	serveBackupOnly bool

	printVersion bool
)

func init() {
	flag.StringVar(&masterHost, "master", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&clusterName, "etcd-cluster", "", "")
	flag.StringVar(&listenAddr, "listen", "0.0.0.0:19999", "")
	flag.BoolVar(&printVersion, "version", false, "Show version and quit")
	flag.BoolVar(&serveBackupOnly, "serve-backup-only", false, "feature gate for simpler service to serve backup only")

	flag.Parse()

	namespace = os.Getenv(constants.EnvOperatorPodNamespace)
	if len(namespace) == 0 {
		namespace = "default"
	}
}

func main() {
	if printVersion {
		fmt.Println("etcd-backup", version.Version)
		os.Exit(0)
	}

	if len(clusterName) == 0 {
		panic("clusterName not set")
	}

	var cs api.ClusterSpec
	sps := os.Getenv(env.ClusterSpec)
	if err := json.Unmarshal([]byte(sps), &cs); err != nil {
		logrus.Fatalf("fail to parse backup policy (%s): %v", sps, err)
	}

	kclient := k8sutil.MustNewKubeClient()
	bk, err := backup.NewBackupController(kclient, clusterName, namespace, cs, listenAddr)
	if err != nil {
		logrus.Fatalf("failed to create backup sidecar: %v", err)
	}

	ctx := context.Background()
	go bk.StartHTTP()
	if !serveBackupOnly {
		go bk.Run()
	}

	<-ctx.Done()
}
