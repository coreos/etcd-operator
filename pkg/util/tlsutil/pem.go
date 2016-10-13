package tlsutil

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

const (
	rsaPrivateKeyType   = "RSA PRIVATE KEY"
	x509CertificateType = "CERTIFICATE"
)

func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  rsaPrivateKeyType,
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return pem.EncodeToMemory(&block)
}

func DecodePrivateKeyPEM(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("Could not decode pem data for private key")
	}
	if block.Type != rsaPrivateKeyType {
		return nil, fmt.Errorf("Decoded block has expected Type: %s", block.Type)
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func EncodeCertificatePEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  x509CertificateType,
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}

func DecodeCertificatePEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("Could not decode pem data for private key")
	}
	if block.Type != x509CertificateType {
		return nil, fmt.Errorf("Decoded block has expected Type: %s", block.Type)
	}
	return x509.ParseCertificate(block.Bytes)
}
