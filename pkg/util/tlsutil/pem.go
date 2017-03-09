package tlsutil

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
)

// Private Key
func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return pem.EncodeToMemory(&block)
}

func DecodePrivateKeyPEMFile(pemPath string) (*rsa.PrivateKey, error) {
	pemBytes, err := ioutil.ReadFile(pemPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemBytes)
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// Certificate
func EncodeCertificatePEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}

func DecodeCertificatePEMFile(pemPath string) (*x509.Certificate, error) {
	pemBytes, err := ioutil.ReadFile(pemPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemBytes)
	return x509.ParseCertificate(block.Bytes)
}
