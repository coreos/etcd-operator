package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/pkg/transport"
	certutil "k8s.io/client-go/util/cert"
)

const (
	certDir        = "/certs"
	caCertFile     = "ca.crt"
	caKeyFile      = "ca.key"
	clientCertFile = "etcd-member.crt"
	clientKeyFile  = "etcd-member.key"
)

var (
	caCert *x509.Certificate
	caKey  *rsa.PrivateKey

	podIP    string
	peerURLs string
)

func main() {
	podIP = os.Getenv("POD_IP")
	if len(podIP) == 0 {
		handleErr(errors.New("No POD_IP set"))
	}

	peerURLs = os.Getenv("PEER_URLS")
	if len(peerURLs) == 0 {
		handleErr(errors.New("No PEER_URLS set"))
	}

	caCertPath := filepath.Join(certDir, caCertFile)
	ensurePathExists(caCertPath)

	caKeyPath := filepath.Join(certDir, caKeyFile)
	ensurePathExists(caKeyPath)

	certs, err := certutil.CertsFromFile(caCertPath)
	handleErr(err)
	caCert = certs[0]

	caKeyBlob, err := certutil.PrivateKeyFromFile(caKeyPath)
	handleErr(err)

	var ok bool
	caKey, ok = caKeyBlob.(*rsa.PrivateKey)
	if !ok {
		handleErr(errors.New("Could not convert private key"))
	}

	createClientPair()
	createCertPair("peer")
	createCertPair("server")

	addMember()
}

func addMember() {
	peerURLs := strings.Split(peerURLs, ",")

	clientCfg := clientv3.Config{
		Endpoints:   peerURLs,
		DialTimeout: 5 * time.Second,
	}

	newMemberURL := fmt.Sprintf("http://%s:2379", podIP)
	if strings.HasPrefix(peerURLs[0], "https") {
		etcdTLSInfo, err := transport.TLSInfo{
			TrustedCAFile: filepath.Join(certDir, caCertFile),
			CertFile:      filepath.Join(certDir, clientCertFile),
			KeyFile:       filepath.Join(certDir, clientKeyFile),
		}.ClientConfig()
		handleErr(err)
		clientCfg.TLS = etcdTLSInfo
		newMemberURL = fmt.Sprintf("https://%s:2379", podIP)
	}

	etcdcli, err := clientv3.New(clientCfg)
	handleErr(err)
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	res, err := etcdcli.MemberAdd(ctx, []string{newMemberURL})
	handleErr(err)

	peerURLs = append(peerURLs, newMemberURL)
	if !reflect.DeepEqual(res.Member.PeerURLs, peerURLs) {
		handleErr(fmt.Errorf("Response from MemberAdd (%v) did not match %v", res.Member.PeerURLs, peerURLs))
	}

	cancel()
}

func createCertPair(filename string) {
	key, err := certutil.NewPrivateKey()
	handleErr(err)

	cfg := certutil.Config{
		CommonName:   fmt.Sprintf("etcd-%s-%s", podIP, filename),
		Organization: []string{"etcd"},
		Usages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		AltNames: certutil.AltNames{
			DNSNames: []string{"localhost"},
			IPs: []net.IP{
				net.ParseIP(podIP),
				net.ParseIP("127.0.0.1"),
			},
		},
	}
	cert, err := certutil.NewSignedCert(cfg, key, caCert, caKey)
	handleErr(err)

	rootPath := filepath.Join(certDir, filename)

	err = certutil.WriteCert(rootPath+".crt", certutil.EncodeCertPEM(cert))
	handleErr(err)

	err = certutil.WriteKey(rootPath+".key", certutil.EncodePrivateKeyPEM(key))
	handleErr(err)
}

func createClientPair() {
	key, err := certutil.NewPrivateKey()
	handleErr(err)

	cfg := certutil.Config{
		CommonName:   fmt.Sprintf("etcd-%s-client", podIP),
		Organization: []string{"etcd"},
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		AltNames:     certutil.AltNames{},
	}
	cert, err := certutil.NewSignedCert(cfg, key, caCert, caKey)
	handleErr(err)

	rootPath := filepath.Join(certDir, clientCertFile)

	err = certutil.WriteCert(rootPath+".crt", certutil.EncodeCertPEM(cert))
	handleErr(err)

	err = certutil.WriteKey(rootPath+".key", certutil.EncodePrivateKeyPEM(key))
	handleErr(err)
}

func ensurePathExists(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		handleErr(fmt.Errorf("%s does not exist", path))
	}
}

func handleErr(err error) {
	if err != nil {
		fmt.Printf("Unexpected error: %v\n", err)
		os.Exit(1)
	}
}
