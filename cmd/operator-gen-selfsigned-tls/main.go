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
	clusterName, clusterNamespace string
	peerSecretName                string
	//Certificate Authority
	peerCACfg tlsutil.CACertConfig
	//Node Server Certificates
	peerCfg                          tlsutil.CertConfig
	peerCertDuration, peerCADuration time.Duration
	prettyPrint                      bool
	outputDir                        string
)

func init() {
	flag.StringVar(&clusterName, "cluster-name", "", "name of etcd operator cluster to generate tls certs for")
	flag.StringVar(&clusterNamespace, "cluster-namespace", "", "namespace of etcd operator cluster is running in")

	flag.StringVar(&peerSecretName, "peer-secret-name", "etcd-operator-peer-tls", "name of the generated generic node identity secret")
	flag.StringVar(&peerCACfg.CommonName, "peer-ca-cn", "etcd-operator-peer-ca", "common name of self-signed etcd peer certificate authority")
	flag.StringVar(&peerCACfg.Organization, "peer-ca-org", "etcd operator", "organization of self-signed ca cert")

	flag.StringVar(&peerCfg.CommonName, "peer-node-server-cn", "etcd-node-peer", "common name of self-signed etcd peer server certificate")

	flag.DurationVar(&peerCADuration, "peer-ca-duration", defaultCADuration, "duration of self-signed peer certificate authority")

	flag.DurationVar(&peerCertDuration, "peer-cert-duration", defaultCertDuration, "duration of self-signed peer certificates")
	flag.StringVar(&outputDir, "output-directory", "", "path to output directory for tls files and kubernetes secrets. defaults to --cluster-name")

	flag.Parse()

	if clusterName == "" {
		logrus.Fatalf("--cluster-name parameter is required")
	}
	if clusterNamespace == "" {
		logrus.Fatalf("--cluster-namespace parameter is required")
	}
	if outputDir == "" {
		outputDir = "./" + clusterName
	}

	now := time.Now().UTC()
	peerCACfg.NotBefore = now
	peerCACfg.NotAfter = now.Add(peerCADuration).UTC()
	peerCfg.NotBefore = now
	peerCfg.NotAfter = now.Add(peerCertDuration).UTC()
	peerCfg.Organization = peerCACfg.Organization
	peerCfg.DNSNames = []string{fmt.Sprintf("*.%s.%s.svc.cluster.local", clusterName, clusterNamespace)}
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

	// Initialize self-signed CA signers
	peerCASigner := tlsutil.NewLocalCertificateSigner(peerCAKey, nil)

	// Mint CA Identities
	peerCACert, err := tlsutil.NewCAIdentity(peerCACfg, peerCASigner)
	if err != nil {
		logrus.Fatalf("error generating self-signed peer ca identity: %v", err)
	}

	/* GENERIC ETCD NODE IDENTITY */
	// Generate etcd node public/private key pairs
	peerKey, err := tlsutil.NewPrivateKey()
	if err != nil {
		logrus.Fatalf("error generating node peer-interface private key: %v", err)
	}
	peerCfg.PublicKey = peerKey.Public()

	// Initialize node signers
	peerSigner := tlsutil.NewLocalCertificateSigner(peerCAKey, peerCACert)

	// Mint node server identity (all nodes have same identity)
	peerCert, err := tlsutil.NewSignedPeerIdentity(peerCfg, peerSigner)
	if err != nil {
		logrus.Fatalf("error generating node peer-interface identity: %v", err)
	}

	/* PUT IT ALL IN KUBERNETES SECRETS */
	// etcd ca (peer and client)
	peerSecret, err := k8sutil.NewEtcdPeerTLSSecret(peerSecretName, peerKey, peerCert, peerCACert)
	if err != nil {
		logrus.Fatalf("error generating self-signed node peer secret: %v", err)
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
			keyName:  "peer-ca-key.pem",
			key:      peerCAKey,
			certName: "peer-ca-crt.pem",
			cert:     peerCACert,
		}, {
			keyName:  "peer-key.pem",
			key:      peerKey,
			certName: "peer-crt.pem",
			cert:     peerCert,
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
			name: peerSecretName + ".json",
			obj:  peerSecret,
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
