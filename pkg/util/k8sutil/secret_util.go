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
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
)

var (
	nodeDuration         = 10 * 365 * 24 * time.Hour
	clientDuration       = 10 * 365 * 24 * time.Hour
	selfSignedCADuration = 10 * 365 * 24 * time.Hour
)

func NewEtcdPeerTLSSecret(secretName string, peerKey *rsa.PrivateKey, peerCert, peerCACert *x509.Certificate) (*v1.Secret, error) {
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
			"peer-key.pem":    tlsutil.EncodePrivateKeyPEM(peerKey),
			"peer-crt.pem":    tlsutil.EncodeCertificatePEM(peerCert),
			"peer-ca-crt.pem": tlsutil.EncodeCertificatePEM(peerCACert),
		},
	}
	return &tlsSecret, nil
}
