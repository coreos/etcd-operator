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

package k8sutil

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/tlsutil"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

var (
	nodeDuration         = 10 * 365 * 24 * time.Hour
	clientDuration       = 10 * 365 * 24 * time.Hour
	selfSignedCADuration = 10 * 365 * 24 * time.Hour
)

// TODO(chom): in eventual case, users should have option to configure secret storage for
// peer and client CA's separately, this will do for now.
func NewEtcdNodeIdentitySecret(secretName string, clientKey, peerKey *rsa.PrivateKey, clientCert, peerCert *x509.Certificate) (*v1.Secret, error) {

	tlsSecret := v1.Secret{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: secretName,
			Labels: map[string]string{
				"app": "etcd",
			},
		},
		Data: map[string][]byte{
			NodePeerKeyName:  tlsutil.EncodePrivateKeyPEM(peerKey),
			NodePeerCertName: tlsutil.EncodeCertificatePEM(peerCert),

			NodeClientKeyName:  tlsutil.EncodePrivateKeyPEM(clientKey),
			NodeClientCertName: tlsutil.EncodeCertificatePEM(clientCert),
		},
	}
	return &tlsSecret, nil
}

// Make TLS credentials for accessing etcd client interface
func NewEtcdClientIdentitySecret(secretName string, clientKey *rsa.PrivateKey, clientCert *x509.Certificate) (*v1.Secret, error) {

	tlsSecret := v1.Secret{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: secretName,
			Labels: map[string]string{
				"app": "etcd",
			},
		},
		Data: map[string][]byte{
			EtcdClientKeyName:  tlsutil.EncodePrivateKeyPEM(clientKey),
			EtcdClientCertName: tlsutil.EncodeCertificatePEM(clientCert),
		},
	}

	return &tlsSecret, nil
}

//TODO(chom): implement external signer flow(s) to support integrating TLS CA generation w/ existing PKI of various flavors
//This is problem with a very large, open-ended solution set.
func NewSelfSignedClusterCASecret(secretName string, clientCACert, peerCACert *x509.Certificate) (*v1.Secret, error) {

	caSecret := v1.Secret{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: secretName,
			Labels: map[string]string{
				"app": "etcd",
			},
		},
		Data: map[string][]byte{
			PeerCAKeyName:  tlsutil.EncodeCertificatePEM(peerCACert),
			PeerCACertName: tlsutil.EncodeCertificatePEM(clientCACert),
		},
	}
	return &caSecret, nil
}

func getSecret(uri string, restcli rest.Interface) (*v1.Secret, error) {
	b, err := restcli.Get().RequestURI(uri).DoRaw()
	if err != nil {
		return nil, err
	}
	secret := &v1.Secret{}
	if err := json.Unmarshal(b, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func GetEtcdHTTPClientFromSecrets(clientSecretName, caSecretName, ns string, restcli rest.Interface) (*http.Client, error) {
	baseURI := "/apis/coreos.com/v1/namespaces/%s/secrets/%s"

	// Fetch secrets from Kubernetes API
	caSecret, err := getSecret(fmt.Sprintf(baseURI, ns, caSecretName), restcli)
	if err != nil {
		return nil, fmt.Errorf("error fetching ca secret: %v", err)
	}
	clientSecret, err := getSecret(fmt.Sprintf(baseURI, ns, clientSecretName), restcli)
	if err != nil {
		return nil, fmt.Errorf("error fetching client secret: %v", err)
	}

	// Retrieve byte arrays from Data map
	caCertBytes, ok := caSecret.Data[ClientCACertName]
	if !ok {
		return nil, fmt.Errorf("could not find %s data key in secret %s", PeerCACertName, caSecretName)
	}

	clientKeyBytes, ok := clientSecret.Data[NodeClientKeyName]
	if !ok {
		return nil, fmt.Errorf("could not find %s data key in secret %s", NodeClientKeyName, clientSecretName)
	}

	clientCertBytes, ok := clientSecret.Data[NodeClientCertName]
	if !ok {
		return nil, fmt.Errorf("could not find %s data key in secret %s", NodeClientCertName, clientSecretName)
	}

	keyPair, err := tls.X509KeyPair(clientKeyBytes, clientCertBytes)
	if err != nil {
		return nil, fmt.Errorf("error creating tls key pair: %v", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCertBytes)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{keyPair},
		RootCAs:      caCertPool,
	}
	tlsConfig.BuildNameToCertificate()
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
		Timeout:   30 * time.Second,
	}

	return client, nil

}
