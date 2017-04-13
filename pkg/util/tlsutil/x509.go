package tlsutil

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math"
	"math/big"
	"time"
)

var (
	Duration365d = time.Hour * 24 * 365
)

const (
	RSAKeySize = 2048
)

type CACertConfig struct {
	CommonName          string
	Organization        string
	NotBefore, NotAfter time.Time
	PublicKey           crypto.PublicKey
}

type CertConfig struct {
	CommonName          string
	Organization        string
	NotBefore, NotAfter time.Time
	DNSNames            []string
	IPAddresses         []string
	PublicKey           crypto.PublicKey
	extKeyUsage         []x509.ExtKeyUsage
}

func NewPrivateKey() (*rsa.PrivateKey, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, RSAKeySize)
	if err != nil {
		return nil, fmt.Errorf("error generating RSA private key: %v", err)
	}
	return privKey, nil
}

// Provides suitable abstraction for local signing and remote signing of certificates
type CertificateSigner func(pubKey crypto.PublicKey, certTmpl *x509.Certificate) (*x509.Certificate, error)

func NewLocalCertificateSigner(caKey *rsa.PrivateKey, caCert *x509.Certificate) CertificateSigner {
	return func(pubKey crypto.PublicKey, certTmpl *x509.Certificate) (*x509.Certificate, error) {
		if caCert == nil {
			// this is the self-signed case
			caCert = certTmpl
		}
		certDERBytes, err := x509.CreateCertificate(rand.Reader, certTmpl, caCert, pubKey, caKey)
		if err != nil {
			return nil, fmt.Errorf("error generating x509 certificate: %v", err)
		}
		cert, err := x509.ParseCertificate(certDERBytes)
		if err != nil {
			panic(fmt.Sprintf("cannot parse x509 certificate that was just generated. this should never happen %v", err))
		}

		return cert, nil
	}
}

//TODO: implement/integrate with remote certificate signers

//Identity Factories
func NewSignedClientIdentity(cfg CertConfig, signer CertificateSigner) (*x509.Certificate, error) {
	cfg.extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	return newSignedIdentity(cfg, signer)
}

func NewSignedServerIdentity(cfg CertConfig, signer CertificateSigner) (*x509.Certificate, error) {
	cfg.extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	return newSignedIdentity(cfg, signer)
}

func NewSignedPeerIdentity(cfg CertConfig, signer CertificateSigner) (*x509.Certificate, error) {
	cfg.extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	return newSignedIdentity(cfg, signer)
}

func NewCAIdentity(cfg CACertConfig, signer CertificateSigner) (*x509.Certificate, error) {
	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: []string{cfg.Organization},
		},
		NotBefore:             cfg.NotBefore,
		NotAfter:              cfg.NotAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA: true,
	}
	caCert, err := signer(cfg.PublicKey, &tmpl)
	if err != nil {
		return nil, fmt.Errorf("error generating self-signed CA certificate: %v", err)
	}

	return caCert, nil
}

func newSignedIdentity(cfg CertConfig, signer CertificateSigner) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: []string{cfg.Organization},
		},
		DNSNames:     cfg.DNSNames,
		SerialNumber: serial,
		NotBefore:    cfg.NotBefore,
		NotAfter:     cfg.NotAfter,
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.extKeyUsage,
	}

	cert, err := signer(cfg.PublicKey, &certTmpl)
	if err != nil {
		return nil, fmt.Errorf("error signing certificate template: %v", err)
	}

	return cert, nil
}
