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
	masterHost  string
	clusterName string
	listenAddr  string
	namespace   string
	// serveBackupOnly flag indicates that this backup service only serves
	// http backup requests.
	serveBackupOnly bool

	printVersion bool
)

func init() {
	flag.StringVar(&masterHost, "master", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&clusterName, "etcd-cluster", "", "")
	flag.StringVar(&listenAddr, "listen", "0.0.0.0:19999", "")
	flag.BoolVar(&printVersion, "version", false, "Show version and quit")

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

	bp, tls, err := parseSpecsFromEnv()
	if err != nil {
		logrus.Fatalf("failed to parse specs from environment: %v", err)
	}
	bc := &backup.BackupControllerConfig{
		Kubecli:      k8sutil.MustNewKubeClient(),
		ListenAddr:   listenAddr,
		ClusterName:  clusterName,
		Namespace:    namespace,
		TLS:          tls,
		BackupPolicy: bp,
	}

	bk, err := backup.NewBackupController(bc)
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

// parseSpecsFromEnv parses ClusterSpec and BackupSpec from env if any.
func parseSpecsFromEnv() (*api.BackupPolicy, *api.TLSPolicy, error) {
	var (
		bp api.BackupPolicy
		cs api.ClusterSpec
	)

	sps := os.Getenv(env.ClusterSpec)
	if err := json.Unmarshal([]byte(sps), &cs); err != nil {
		return nil, nil, fmt.Errorf("failed to parse cluster spec (%s): %v", sps, err)
	}

	if ebs := os.Getenv(env.BackupSpec); len(ebs) != 0 {
		// set serveBackupOnly to true if backup spec exists.
		serveBackupOnly = true
		var bs api.BackupSpec
		if err := json.Unmarshal([]byte(ebs), &bs); err != nil {
			return nil, nil, fmt.Errorf("failed to parse backup spec (%s): %v", ebs, err)
		}
		bp.StorageType = api.BackupStorageType(bs.StorageType)
		switch bp.StorageType {
		case api.BackupStorageTypeS3:
			bp.StorageSource.S3 = bs.BackupStorageSource.S3
		default:
			return nil, nil, fmt.Errorf("unknown backup type (%v)", bp.StorageType)
		}
	} else {
		if cs.Backup == nil {
			return nil, nil, fmt.Errorf("backup policy not found")
		}
		bp = *cs.Backup
	}

	return &bp, cs.TLS, nil
}
