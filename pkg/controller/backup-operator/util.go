// Copyright 2017 The etcd-operator Authors
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

package controller

import (
	"crypto/tls"
	"fmt"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/client-go/kubernetes"
)

func generateTLSConfig(kubecli kubernetes.Interface, clientTLSSecret, namespace string) (*tls.Config, error) {
	var tlsConfig *tls.Config
	if len(clientTLSSecret) != 0 {
		d, err := k8sutil.GetTLSDataFromSecret(kubecli, namespace, clientTLSSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to get TLS data from secret (%v): %v", clientTLSSecret, err)
		}
		tlsConfig, err = etcdutil.NewTLSConfig(d.CertData, d.KeyData, d.CAData)
		if err != nil {
			return nil, fmt.Errorf("failed to constructs tls config: %v", err)
		}
	}
	return tlsConfig, nil
}

func isPeriodicBackup(ebSpec *api.BackupSpec) bool {
	if ebSpec.BackupPolicy != nil {
		return ebSpec.BackupPolicy.BackupIntervalInSecond != 0
	}
	return false
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
