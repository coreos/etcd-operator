package tlsutil

import (
	"crypto/rand"
	"crypto/rsa"
	"io/ioutil"
)

const (
	RSAKeySize = 2048
)

func NewPrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, RSAKeySize)
}

func writePrivateKeyToFile(keyPath string, key *rsa.PrivateKey) error {
	data := EncodePrivateKeyPEM(key)
	return ioutil.WriteFile(keyPath, data, tlsFileMode)
}

func readPrivateKeyFromFile(keyPath string) (*rsa.PrivateKey, error) {
	data, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	return DecodePrivateKeyPEM(data)
}
