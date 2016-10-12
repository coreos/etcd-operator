package tlsutil

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"os"
)

const (
	tlsFileMode = 0600
)

// Parse from file if exists, else create new and write to file
func InitializePrivateKeyFile(keyPath string) (*rsa.PrivateKey, error) {
	//attempt to read private key from file
	key, err := readPrivateKeyFromFile(keyPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error reading private key file %s : %v", keyPath, err)
		}
		//create new private key
		key, err = NewPrivateKey()
		if err != nil {
			return nil, err
		}
		//save private key to file
		if err := writePrivateKeyToFile(keyPath, key); err != nil {
			return nil, err
		}
	}
	//parse private key from file
	return key, nil
}

type NewCertificateFunc func(privKey *rsa.PrivateKey) (*x509.Certificate, error)

// Parse from cert file if exists, else create new certificate and write to file
func InitializeCertificateFile(certPath string, privKey *rsa.PrivateKey, newCert NewCertificateFunc) (*x509.Certificate, error) {
	//attempt to parse private cert from file
	cert, err := readCertificateFromFile(certPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error reading cert file %s : %v", certPath, err)
		}
		//create new ca cert
		cert, err = newCert(privKey)
		if err != nil {
			return nil, err
		}
		//save ca cert to file
		if err := writeCertificateToFile(certPath, cert); err != nil {
			return nil, err
		}
	}
	return cert, nil
}
