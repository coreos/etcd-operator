package tlsutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io/ioutil"
	"math/big"
	"time"
)

type CACertConfig struct {
	CommonName   string
	Organization string
	Duration     time.Duration
}

type ServerCertConfig struct {
	CommonName  string
	DNSNames    []string
	IPAddresses []string
	Duration    time.Duration
}

type ClientCertConfig struct {
	CommonName  string
	DNSNames    []string
	IPAddresses []string
	Duration    time.Duration
}

func writeCertificateToFile(certPath string, cert *x509.Certificate) error {
	data := EncodeCertificatePEM(cert)
	return ioutil.WriteFile(certPath, data, tlsFileMode)
}

func readCertificateFromFile(certPath string) (*x509.Certificate, error) {
	data, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	return DecodeCertificatePEM(data)
}

func NewCACertificate(cfg CACertConfig, key, parentKey *rsa.PrivateKey, parentCert *x509.Certificate) (*x509.Certificate, error) {
	if cfg.Duration <= 0 {
		return nil, errors.New("Cert duration must not be negative or zero.")
	}
	cert := &x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: []string{cfg.Organization},
		},
		NotBefore:             time.Now().UTC(),
		NotAfter:              time.Now().Add(cfg.Duration).UTC(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA: true,
	}
	if parentKey == nil {
		parentKey = key
	}
	if parentCert == nil {
		parentCert = cert
	}
	certDERBytes, err := x509.CreateCertificate(rand.Reader, cert, parentCert, key.Public(), parentKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}
