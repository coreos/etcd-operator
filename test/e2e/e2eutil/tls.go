// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Generate a self-signed X.509 certificate for a TLS server. Outputs to
// 'cert.pem' and 'key.pem' and will overwrite existing files.

package e2eutil

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// PrepareTLS creates all the required tls certs for a given clusterName.
func PrepareTLS(clusterName, namespace, memberPeerTLSSecret, memberClientTLSSecret, operatorClientTLSSecret string) error {
	err := PreparePeerTLSSecret(clusterName, namespace, memberPeerTLSSecret)
	if err != nil {
		return fmt.Errorf("failed to prepare peer TLS secret: %v", err)
	}
	certsDir, err := ioutil.TempDir("", "etcd-operator-tls-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(certsDir)
	err = PrepareClientTLSSecret(certsDir, clusterName, namespace, memberClientTLSSecret, operatorClientTLSSecret)
	if err != nil {
		return fmt.Errorf("failed to prepare client TLS secret: %v", err)
	}
	return nil
}

func PreparePeerTLSSecret(clusterName, ns, secretName string) error {
	dir, err := ioutil.TempDir("", "etcd-operator-tls-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	certPath := filepath.Join(dir, "peer.crt")
	keyPath := filepath.Join(dir, "peer.key")
	caPath := filepath.Join(dir, "peer-ca.crt")
	hosts := []string{
		fmt.Sprintf("*.%s.%s.svc", clusterName, ns),
		// Due to issue https://github.com/coreos/etcd/issues/8797,
		// we need to provide FQDN in certs at the moment.
		fmt.Sprintf("*.%s.%s.svc.cluster.local", clusterName, ns),
	}

	err = prepareTLSCerts(certPath, keyPath, caPath, hosts)
	if err != nil {
		return err
	}
	cmd := exec.Command("kubectl", "-n", ns, "create", "secret", "generic", secretName,
		fmt.Sprintf("--from-file=%s", caPath),
		fmt.Sprintf("--from-file=%s", certPath),
		fmt.Sprintf("--from-file=%s", keyPath),
	)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to create tls secret. stdout/stderr: %s", stdoutStderr)
		return fmt.Errorf("failed to create tls secret: %v", err)
	}
	return nil
}

func PrepareClientTLSSecret(dir, clusterName, ns, mSecret, oSecret string) error {
	mCertPath := filepath.Join(dir, "server.crt")
	mKeyPath := filepath.Join(dir, "server.key")
	oCAPath := filepath.Join(dir, "etcd-client-ca.crt")
	mHosts := []string{
		fmt.Sprintf("*.%s.%s.svc", clusterName, ns),
		fmt.Sprintf("%s-client.%s.svc", clusterName, ns),
		"localhost",
	}

	err := prepareTLSCerts(mCertPath, mKeyPath, oCAPath, mHosts)
	if err != nil {
		return err
	}

	oCertPath := filepath.Join(dir, "etcd-client.crt")
	oKeyPath := filepath.Join(dir, "etcd-client.key")
	mCAPath := filepath.Join(dir, "server-ca.crt")
	oHosts := []string{""}

	err = prepareTLSCerts(oCertPath, oKeyPath, mCAPath, oHosts)
	if err != nil {
		return err
	}

	cmd := exec.Command("kubectl", "-n", ns, "create", "secret", "generic", mSecret,
		fmt.Sprintf("--from-file=%s", mCAPath),
		fmt.Sprintf("--from-file=%s", mCertPath),
		fmt.Sprintf("--from-file=%s", mKeyPath),
	)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to create tls secret. stdout/stderr: %s", stdoutStderr)
		return fmt.Errorf("failed to create tls secret: %v", err)
	}

	cmd = exec.Command("kubectl", "-n", ns, "create", "secret", "generic", oSecret,
		fmt.Sprintf("--from-file=%s", oCAPath),
		fmt.Sprintf("--from-file=%s", oCertPath),
		fmt.Sprintf("--from-file=%s", oKeyPath),
	)
	stdoutStderr, err = cmd.CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to create tls secret. stdout/stderr: %s", stdoutStderr)
		return fmt.Errorf("failed to create tls secret: %v", err)
	}
	return nil
}

func prepareTLSCerts(certPath, keyPath, caPath string, hosts []string) error {
	err := prepareKeyAndCert(certPath, keyPath, hosts)
	if err != nil {
		return err
	}
	b, err := ioutil.ReadFile(certPath)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(caPath, b, 0644)
}

// prepareKeyAndCert creates self-signed self-CA x509 key and cert file.
// The files are written as given certPath and keyPath respectively.
// hosts: Comma-separated hostnames and IPs to generate a certificate for.
func prepareKeyAndCert(certPath, keyPath string, hosts []string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %v", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(2 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Etcd Operator Example"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		return fmt.Errorf("Failed to create certificate: %v", err)
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("failed to open cert.pem for writing: %v", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open key.pem for writing: %v", err)
	}
	pb, err := pemBlockForKey(priv)
	if err != nil {
		return err
	}
	pem.Encode(keyOut, pb)
	keyOut.Close()
	return nil
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func pemBlockForKey(priv interface{}) (*pem.Block, error) {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}, nil
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, fmt.Errorf("Unable to marshal ECDSA private key: %v", err)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}, nil
	default:
		return nil, errors.New("unknown private key type")
	}
}
