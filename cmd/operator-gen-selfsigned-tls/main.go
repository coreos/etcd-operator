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
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/tlsutil"

	"github.com/Sirupsen/logrus"
)

const (
	defaultCADuration   = 10 * 365 * 24 * time.Hour
	defaultCertDuration = 3 * 365 * 24 * time.Hour
)

var (
	clusterName, clusterNamespace                  string
	caSecretName, nodeSecretName, clientSecretName string
	//Certificate Authority
	peerCACfg, clientCACfg tlsutil.CACertConfig
	//Node Server Certificates
	nodePeerServerCfg, nodeClientServerCfg, etcdClientCfg tlsutil.CertConfig
	//Node Client Certificate (for accessing etcd client interface)
	nodeClientClientCfg tlsutil.CertConfig

	peerCertDuration, clientCertDuration time.Duration
	prettyPrint                          bool
	outputDir                            string
)

func init() {
	flag.StringVar(&clusterName, "cluster-name", "default-cluster", "name of etcd operator cluster to generate tls certs for")
	flag.StringVar(&clusterNamespace, "cluster-namespace", "default", "namespace of etcd operator cluster is running in")

	flag.StringVar(&caSecretName, "ca-secret-name", "etcd-operator-ca", "name of the generated self-signed CA secret")
	flag.StringVar(&nodeSecretName, "node-secret-name", "etcd-operator-generic-node-tls", "name of the generated generic node identity secret")
	flag.StringVar(&clientSecretName, "client-secret-name", "etcd-operator-client", "name of the generated etcd client identity secret")
	flag.StringVar(&peerCACfg.CommonName, "peer-ca-cn", "etcd-operator-peer-ca", "common name of self-signed etcd peer certificate authority")
	flag.StringVar(&clientCACfg.CommonName, "client-ca-cn", "etcd-operator-client-ca", "common name of self-signed etcd client certificate authority")

	flag.StringVar(&nodePeerServerCfg.CommonName, "peer-node-server-cn", "etcd-node-peer", "common name of self-signed etcd peer server certificate")
	flag.StringVar(&nodeClientServerCfg.CommonName, "client-node-server-cn", "etcd-node-client", "common name of self-signed etcd client server certificate")
	flag.StringVar(&nodeClientClientCfg.CommonName, "client-node-client-cn", "etcd-client", "common name of self-signed etcd client (interface) client certificate. (used for accessing client interface)")

	flag.DurationVar(&peerCACfg.Duration, "peer-ca-duration", defaultCADuration, "duration of self-signed peer certificate authority")
	flag.DurationVar(&clientCACfg.Duration, "client-ca-duration", defaultCADuration, "duration of self-signed client certificate authority")

	flag.DurationVar(&peerCertDuration, "peer-cert-duration", defaultCertDuration, "duration of self-signed peer certificates")
	flag.DurationVar(&clientCertDuration, "client-cert-duration", defaultCertDuration, "duration of self-signed client certificates")

	flag.StringVar(&outputDir, "output-directory", "./etcd-operator-selfsigned-tls", "path to output directory for tls files and kubernetes secrets")

	flag.Parse()

	nodePeerServerCfg.Duration = peerCertDuration
	nodeClientServerCfg.Duration = clientCertDuration
	etcdClientCfg.Duration = clientCertDuration

	nodePeerServerCfg.Organization = peerCACfg.Organization
	nodeClientServerCfg.Organization = clientCACfg.Organization
	etcdClientCfg.Organization = clientCACfg.Organization

	nodeClientServerCfg.DNSNames = []string{clusterName, fmt.Sprintf("%s.%s.svc.cluster.local", clusterName, clusterNamespace)}
	nodePeerServerCfg.DNSNames = []string{fmt.Sprintf("*.%s.svc.cluster.local", clusterNamespace)}

}

type keyPairFile struct {
	keyName, certName string
	key               *rsa.PrivateKey
	cert              *x509.Certificate
}

type jsonManifestFile struct {
	name string
	obj  interface{}
}

func main() {

	/* SELF-SIGNED CERTIFICATE AUTHORITY */

	// Generate etcd node public/private key pairs
	// For both peer and client
	peerCAKey, err := tlsutil.NewPrivateKey()
	if err != nil {
		logrus.Fatalf("failed generating peer CA private key: %v", err)
	}
	peerCACfg.PublicKey = peerCAKey.Public()

	clientCAKey, err := tlsutil.NewPrivateKey()
	if err != nil {
		logrus.Fatalf("failed generating client CA private key: %v", err)
	}
	clientCACfg.PublicKey = clientCAKey.Public()

	// Initialize self-signed CA signers
	peerSigner := tlsutil.NewLocalCertificateSigner(peerCAKey, nil)
	clientSigner := tlsutil.NewLocalCertificateSigner(clientCAKey, nil)

	// Mint CA Identities
	peerCACert, err := tlsutil.NewCAIdentity(peerCACfg, peerSigner)
	if err != nil {
		logrus.Fatalf("error generating self-signed peer ca identity: %v", err)
	}
	clientCACert, err := tlsutil.NewCAIdentity(clientCACfg, clientSigner)
	if err != nil {
		logrus.Fatalf("error generating self-signed client ca identity: %v", err)
	}

	/* GENERIC ETCD NODE IDENTITY */

	// Generate etcd node public/private key pairs
	nodePeerKey, err := tlsutil.NewPrivateKey()
	if err != nil {
		logrus.Fatalf("error generating node peer-interface private key: %v", err)
	}
	nodePeerServerCfg.PublicKey = nodePeerKey.Public()

	nodeClientKey, err := tlsutil.NewPrivateKey()
	if err != nil {
		logrus.Fatalf("error generating node client-interface private key: %v", err)
	}
	nodeClientServerCfg.PublicKey = nodeClientKey.Public()

	// Initialize node signers
	nodePeerSigner := tlsutil.NewLocalCertificateSigner(peerCAKey, peerCACert)
	nodeClientSigner := tlsutil.NewLocalCertificateSigner(clientCAKey, clientCACert)

	// Mint node server identity (all nodes have same identity)
	nodePeerCert, err := tlsutil.NewSignedServerIdentity(nodePeerServerCfg, nodePeerSigner)
	if err != nil {
		logrus.Fatalf("error generating node peer-interface identity: %v", err)
	}
	nodeClientCert, err := tlsutil.NewSignedServerIdentity(nodeClientServerCfg, nodeClientSigner)
	if err != nil {
		logrus.Fatalf("error generating node client-interface identity: %v", err)
	}

	/* ETCD CLIENT IDENTITY */

	// Generate etcd client public/private key pair
	etcdClientKey, err := tlsutil.NewPrivateKey()
	if err != nil {
		logrus.Fatalf("error generating etcd client private key: %v", err)
	}
	etcdClientCfg.PublicKey = etcdClientKey.Public()

	// Initialize etcd client signer
	etcdClientSigner := tlsutil.NewLocalCertificateSigner(clientCAKey, clientCACert)

	// Mint etcd client identity (required by etcd-operator to access etcd client interface)
	etcdClientCert, err := tlsutil.NewSignedClientIdentity(etcdClientCfg, etcdClientSigner)

	/* PUT IT ALL IN KUBERNETES SECRETS */

	// etcd ca (peer and client)
	caSecret, err := k8sutil.NewSelfSignedClusterCASecret(caSecretName, clientCACert, peerCACert)
	if err != nil {
		logrus.Fatalf("error generating self-signed cluster ca secret: %v", err)
	}

	// etcd node server (peer and client)
	nodeSecret, err := k8sutil.NewEtcdNodeIdentitySecret(nodeSecretName, nodeClientKey,
		nodePeerKey, nodeClientCert, nodePeerCert)
	if err != nil {
		logrus.Fatalf("error generating node identity secret: %v", err)
	}

	// etcd client
	clientSecret, err := k8sutil.NewEtcdClientIdentitySecret(clientSecretName, etcdClientKey, etcdClientCert)
	if err != nil {
		logrus.Fatalf("error generating etcd client identity secret: %v", err)
	}

	/* INITIALIZE OUTPUT DIRECTORY STRUCTORY */

	if err := os.Mkdir(outputDir, 0700); err != nil {
		logrus.Fatalf("error creating output directory %s : %v", outputDir, err)
	}

	for _, subDir := range []string{"pem", "kubernetes"} {
		if err := os.Mkdir(filepath.Join(outputDir, subDir), 0700); err != nil {
			logrus.Fatalf("error creating output directory %s: %v", outputDir, err)
		}
	}

	/* WRITE OUTPUT FILES */
	logrus.Infof("Writing output files...")
	// Write public/private keys
	keyPairs := []keyPairFile{
		{
			keyName:  k8sutil.PeerCAKeyName,
			key:      peerCAKey,
			certName: k8sutil.PeerCACertName,
			cert:     peerCACert,
		}, {
			keyName:  k8sutil.ClientCAKeyName,
			key:      clientCAKey,
			certName: k8sutil.ClientCACertName,
			cert:     clientCACert,
		}, {
			keyName:  k8sutil.NodePeerKeyName,
			key:      nodePeerKey,
			certName: k8sutil.NodePeerCertName,
			cert:     nodePeerCert,
		}, {
			keyName:  k8sutil.NodeClientKeyName,
			key:      nodeClientKey,
			certName: k8sutil.NodeClientCertName,
			cert:     nodeClientCert,
		}, {
			keyName:  k8sutil.EtcdClientKeyName,
			key:      etcdClientKey,
			certName: k8sutil.EtcdClientCertName,
			cert:     etcdClientCert,
		},
	}

	for _, kp := range keyPairs {
		keyPath := filepath.Join(outputDir, "pem", kp.keyName)
		certPath := filepath.Join(outputDir, "pem", kp.certName)
		if err := ioutil.WriteFile(keyPath, tlsutil.EncodePrivateKeyPEM(kp.key), 0600); err != nil {
			logrus.Fatalf("error writing private key file %s: %v", keyPath, err)
		}
		if err := ioutil.WriteFile(certPath, tlsutil.EncodeCertificatePEM(kp.cert), 0600); err != nil {
			logrus.Fatalf("error writing certificate file %s: %v", certPath, err)
		}
		logrus.Infof("\t |-> %s", keyPath)
		logrus.Infof("\t |-> %s", certPath)
	}

	jsonManifests := []jsonManifestFile{
		{
			name: caSecretName + ".json",
			obj:  caSecret,
		}, {
			name: nodeSecretName + ".json",
			obj:  nodeSecret,
		}, {
			name: clientSecretName + ".json",
			obj:  clientSecret,
		},
	}

	for _, jsonManifest := range jsonManifests {
		jsonBytes, err := json.Marshal(jsonManifest.obj)
		if err != nil {
			logrus.Fatalf("error marshalling json: %v", err)
		}
		jsonPath := filepath.Join(outputDir, "kubernetes", jsonManifest.name)
		if err := ioutil.WriteFile(jsonPath, jsonBytes, 0600); err != nil {
			logrus.Fatalf("error writing output file: %v", err)
		}
		logrus.Infof("\t |-> %s\n", jsonPath)
	}
}
