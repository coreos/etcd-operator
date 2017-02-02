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
	"crypto/x509"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/tlsutil"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/api/v1"
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
			"peer-server-key.pem":  tlsutil.EncodePrivateKeyPEM(peerKey),
			"peer-server-cert.pem": tlsutil.EncodeCertificatePEM(peerCert),

			"client-server-key.pem":  tlsutil.EncodePrivateKeyPEM(clientKey),
			"client-server-cert.pem": tlsutil.EncodeCertificatePEM(clientCert),
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
			"client-key.pem":  tlsutil.EncodePrivateKeyPEM(clientKey),
			"client-cert.pem": tlsutil.EncodeCertificatePEM(clientCert),
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
			"peer-ca-cert.pem":   tlsutil.EncodeCertificatePEM(peerCACert),
			"client-ca-cert.pem": tlsutil.EncodeCertificatePEM(clientCACert),
		},
	}
	return &caSecret, nil
}
