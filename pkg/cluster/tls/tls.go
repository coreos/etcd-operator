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

package tls

import (
	"crypto/tls"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd/pkg/transport"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	etcdCliCertFile = "etcd-crt.pem"
	etcdCliKeyFile  = "etcd-key.pem"
	etcdCliCAFile   = "etcd-ca-crt.pem"
)

func NewTLSConfig(kubecli kubernetes.Interface, ns string, tp spec.TLSPolicy) (*tls.Config, error) {
	secret, err := kubecli.CoreV1().Secrets(ns).Get(tp.Static.OperatorSecret, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	dir, err := ioutil.TempDir("", "etcd-operator-cluster-tls")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	certFile := filepath.Join(dir, etcdCliCertFile)
	err = ioutil.WriteFile(certFile, secret.Data[etcdCliCertFile], 0600)
	if err != nil {
		return nil, err
	}
	keyFile := filepath.Join(dir, etcdCliKeyFile)
	err = ioutil.WriteFile(keyFile, secret.Data[etcdCliKeyFile], 0600)
	if err != nil {
		return nil, err
	}
	caFile := filepath.Join(dir, etcdCliCAFile)
	err = ioutil.WriteFile(caFile, secret.Data[etcdCliCAFile], 0600)
	if err != nil {
		return nil, err
	}

	tlsInfo := transport.TLSInfo{
		CertFile:      certFile,
		KeyFile:       keyFile,
		TrustedCAFile: caFile,
	}
	tlsConfig, err := tlsInfo.ClientConfig()
	if err != nil {
		return nil, err
	}
	return tlsConfig, nil
}

func IsSecureClient(sp spec.ClusterSpec) bool {
	return sp.TLS != nil
}
