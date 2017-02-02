package tlsutil

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
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
	CommonName   string
	Organization string
	Duration     time.Duration
	PublicKey    crypto.PublicKey
}

type CertConfig struct {
	CommonName   string
	Organization string
	Duration     time.Duration
	DNSNames     []string
	IPAddresses  []string
	PublicKey    crypto.PublicKey
	extKeyUsage  []x509.ExtKeyUsage
}

func NewPrivateKey() (*rsa.PrivateKey, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, RSAKeySize)
	if err != nil {
		return nil, fmt.Errorf("error generating RSA private key: %v", err)
	}
	return privKey, nil
}

//Certificate Signers (things which mint identities. Pulling the interface spec out now will make implementing new ones cake)

// This is explictly not a certificate request model
// ie: the singee must divulge private key material to singer
// This is fine for a one-way trust delegation model (CA downwards)
type CertificateSigner func(pubKey crypto.PublicKey, certTmpl *x509.Certificate) (*x509.Certificate, error)

// We have the ca private key in memory already
// Leave caCert nil if this is a self-signed (root of trust) CA
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

//Identity Factories

func NewSignedClientIdentity(cfg CertConfig, signer CertificateSigner) (*x509.Certificate, error) {
	cfg.extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	return newSignedIdentity(cfg, signer)
}

func NewSignedServerIdentity(cfg CertConfig, signer CertificateSigner) (*x509.Certificate, error) {
	cfg.extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	return newSignedIdentity(cfg, signer)
}

//TODO(chom): implement externally-signed CA
func NewCAIdentity(cfg CACertConfig, signer CertificateSigner) (*x509.Certificate, error) {

	if cfg.Duration <= 0 {
		return nil, errors.New("Self-signed CA cert duration must not be negative or zero.")
	}

	tmpl := x509.Certificate{
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

	if cfg.Duration <= 0 {
		return nil, errors.New("Signed cert duration must not be negative or zero.")
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: []string{cfg.Organization},
		},
		DNSNames:     cfg.DNSNames,
		SerialNumber: serial,
		//TODO(chom): NotBefore/NotAfter should probably be configurable externally
		//ie: not a) rely implicitly on system clock and b) assume the duration is meant to start at present time.
		//Both are "generally reasonable but not universally useful" assumptions
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(cfg.Duration).UTC(),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: cfg.extKeyUsage,
	}

	cert, err := signer(cfg.PublicKey, &certTmpl)
	if err != nil {
		return nil, fmt.Errorf("error signing certificate template: %v", err)
	}

	return cert, nil
}
