package controller

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/pkg/analytics"
	"github.com/coreos/kube-etcd-controller/pkg/cluster"
	"github.com/coreos/kube-etcd-controller/pkg/spec"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	"github.com/coreos/kube-etcd-controller/pkg/util/tlsutil"

	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

const (
	tprName = "etcd-cluster.coreos.com"

	//Etcd controller tls file names
	controllerCACertName = "controller-ca-cert.pem"
	controllerCAKeyName  = "controller-ca-key.pem"
	//Etcd cluster tls file name templates
	clusterCACertTemplate = "%s-cluster-ca-cert.pem"
	clusterCAKeyTemplate  = "%s-cluster-ca-key.pem"
)

var (
	supportedPVProvisioners = map[string]struct{}{
		"kubernetes.io/gce-pd":  {},
		"kubernetes.io/aws-ebs": {},
	}
)

type rawEvent struct {
	Type   string
	Object json.RawMessage
}

type Event struct {
	Type   string
	Object *spec.EtcdCluster
}

type Controller struct {
	*Config
	kclient  *unversioned.Client
	clusters map[string]*cluster.Cluster
	caKey    *rsa.PrivateKey
	caCert   *x509.Certificate
}

type Config struct {
	Namespace     string
	MasterHost    string
	TLSInsecure   bool
	TLSConfig     restclient.TLSClientConfig
	PVProvisioner string
	EtcdTLSConfig struct {
		//parameters for generated controller CA
		tlsutil.CACertConfig
		//where the tls assets are stored
		Dir                    string
		SelfSignedControllerCA bool
	}
}

func (c *Config) validate() error {
	if _, ok := supportedPVProvisioners[c.PVProvisioner]; !ok {
		return fmt.Errorf(
			"persistent volume provisioner %s is not supported: options = %v",
			c.PVProvisioner, supportedPVProvisioners,
		)
	}

	return nil
}

//TOOD: Implement this
func (c *Config) remoteSignedControllerCACert(caKey *rsa.PrivateKey) (*x509.Certificate, error) {
	return nil, fmt.Errorf("NOT IMPLEMENTED YET!")
}

func (c *Config) selfSignedControllerCACert(caKey *rsa.PrivateKey) (*x509.Certificate, error) {
	return tlsutil.NewCACertificate(c.EtcdTLSConfig.CACertConfig, caKey, nil, nil)
}

func New(cfg *Config) *Controller {
	if err := cfg.validate(); err != nil {
		panic(err)
	}

	keyPath := filepath.Join(cfg.EtcdTLSConfig.Dir, controllerCAKeyName)
	caKey, err := tlsutil.InitializePrivateKeyFile(keyPath)
	if err != nil {
		panic(err)
	}

	var caCertFunc tlsutil.NewCertificateFunc
	if cfg.EtcdTLSConfig.SelfSignedControllerCA {
		caCertFunc = cfg.selfSignedControllerCACert
	} else {
		caCertFunc = cfg.remoteSignedControllerCACert
	}

	certPath := filepath.Join(cfg.EtcdTLSConfig.Dir, controllerCACertName)
	caCert, err := tlsutil.InitializeCertificateFile(certPath, caKey, caCertFunc)
	if err != nil {
		panic(err)
	}

	kclient := k8sutil.MustCreateClient(cfg.MasterHost, cfg.TLSInsecure, &cfg.TLSConfig)
	if len(cfg.MasterHost) == 0 {
		cfg.MasterHost = k8sutil.MustGetInClusterMasterHost()
	}

	return &Controller{
		Config:   cfg,
		kclient:  kclient,
		clusters: make(map[string]*cluster.Cluster),
		caKey:    caKey,
		caCert:   caCert,
	}
}

func (c *Controller) initializeClusterTLS(clusterName string, clusterSpec *spec.ClusterSpec) (*rsa.PrivateKey, *x509.Certificate, error) {
	// set defaults
	if len(clusterSpec.CACommonName) == 0 {
		clusterSpec.CACommonName = clusterName
	}
	if len(clusterSpec.CAOrganization) == 0 {
		clusterSpec.CAOrganization = c.EtcdTLSConfig.Organization
	}
	if clusterSpec.CADuration == 0 {
		clusterSpec.CADuration = c.EtcdTLSConfig.Duration
	}

	keyPath := filepath.Join(c.EtcdTLSConfig.Dir, fmt.Sprintf(clusterCAKeyTemplate, clusterName))
	caKey, err := tlsutil.InitializePrivateKeyFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	// sign the new cluster certificate with the controller ca key
	clusterCertFunc := func(key *rsa.PrivateKey) (*x509.Certificate, error) {
		certCfg := tlsutil.CACertConfig{
			CommonName:   clusterSpec.CACommonName,
			Organization: clusterSpec.CAOrganization,
			Duration:     clusterSpec.CADuration,
		}

		return tlsutil.NewCACertificate(certCfg, key, c.caKey, c.caCert)
	}
	certPath := filepath.Join(c.EtcdTLSConfig.Dir, fmt.Sprintf(clusterCACertTemplate, clusterName))
	caCert, err := tlsutil.InitializeCertificateFile(certPath, caKey, clusterCertFunc)
	if err != nil {
		return nil, nil, err
	}
	return caKey, caCert, nil
}
func (c *Controller) Run() {
	watchVersion, err := c.initResource()
	if err != nil {
		panic(err)
	}
	log.Println("etcd cluster controller starts running...")

	eventCh, errCh := monitorEtcdCluster(c.MasterHost, c.Namespace, c.kclient.RESTClient.Client, watchVersion)
	for {
		select {
		case event := <-eventCh:
			clusterName := event.Object.ObjectMeta.Name
			switch event.Type {
			case "ADDED":
				clusterSpec := &event.Object.Spec
				caKey, caCert, err := c.initializeClusterTLS(clusterName, clusterSpec)
				if err != nil {
					panic(fmt.Errorf("error adding cluster %s: %v", clusterName, err))
				}

				clusterCfg := &cluster.Config{
					KClient:   c.kclient,
					Name:      clusterName,
					Namespace: c.Namespace,
					Spec:      clusterSpec,
					CAKey:     caKey,
					CACert:    caCert,
				}
				nc := cluster.New(clusterCfg)
				c.clusters[clusterName] = nc
				analytics.ClusterCreated()

				backup := clusterSpec.Backup
				if backup != nil && backup.MaxSnapshot != 0 {
					err := k8sutil.CreateBackupReplicaSetAndService(c.kclient, clusterName, c.Namespace, *backup)
					if err != nil {
						panic(err)
					}
				}
			case "MODIFIED":
				c.clusters[clusterName].Update(&event.Object.Spec)
			case "DELETED":
				c.clusters[clusterName].Delete()
				delete(c.clusters, clusterName)
				analytics.ClusterDeleted()
			}
		case err := <-errCh:
			panic(err)
		}
	}
}

func (c *Controller) findAllClusters() (string, error) {
	log.Println("finding existing clusters...")
	resp, err := k8sutil.ListETCDCluster(c.MasterHost, c.Namespace, c.kclient.RESTClient.Client)
	if err != nil {
		return "", err
	}
	d := json.NewDecoder(resp.Body)
	list := &EtcdClusterList{}
	if err := d.Decode(list); err != nil {
		return "", err
	}
	for _, item := range list.Items {
		nc := cluster.Restore(&cluster.Config{
			KClient:   c.kclient,
			Name:      item.Name,
			Namespace: c.Namespace,
			Spec:      &item.Spec,
		})
		c.clusters[item.Name] = nc

		backup := item.Spec.Backup
		if backup != nil && backup.MaxSnapshot != 0 {
			err := k8sutil.CreateBackupReplicaSetAndService(c.kclient, item.Name, c.Namespace, *backup)
			if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
				panic(err)
			}
		}
	}
	return list.ListMeta.ResourceVersion, nil
}

func (c *Controller) initResource() (string, error) {
	err := c.createTPR()
	if err != nil {
		switch {
		// etcd controller has been initialized before. We don't need to
		// repeat the init process but recover cluster.
		case k8sutil.IsKubernetesResourceAlreadyExistError(err):
			watchVersion, err := c.findAllClusters()
			if err != nil {
				return "", err
			}
			return watchVersion, nil
		default:
			log.Errorf("fail to create TPR: %v", err)
			return "", err
		}
	}
	err = k8sutil.CreateStorageClass(c.kclient, c.PVProvisioner)
	if err != nil {
		log.Errorf("fail to create storage class: %v", err)
		return "", err
	}
	return "0", nil
}

func (c *Controller) createTPR() error {
	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: k8sapi.ObjectMeta{
			Name: tprName,
		},
		Versions: []extensions.APIVersion{
			{Name: "v1"},
		},
		Description: "Managed etcd clusters",
	}
	_, err := c.kclient.ThirdPartyResources().Create(tpr)
	if err != nil {
		return err
	}

	return k8sutil.WaitEtcdTPRReady(c.kclient.Client, 3*time.Second, 30*time.Second, c.MasterHost, c.Namespace)
}

func monitorEtcdCluster(host, ns string, httpClient *http.Client, watchVersion string) (<-chan *Event, <-chan error) {
	eventCh := make(chan *Event)
	// On unexpected error case, controller should exit
	errCh := make(chan error, 1)
	go func() {
		for {
			resp, err := k8sutil.WatchETCDCluster(host, ns, httpClient, watchVersion)
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				errCh <- errors.New("Invalid status code: " + resp.Status)
				return
			}
			log.Printf("watching at %v", watchVersion)
			decoder := json.NewDecoder(resp.Body)
			for {
				ev, err := pollEvent(decoder)
				if err != nil {
					if err == io.EOF { // apiserver will close stream periodically
						break
					}
					errCh <- err
					return
				}
				log.Printf("etcd cluster event: %v %v", ev.Type, ev.Object.Spec)
				watchVersion = ev.Object.ObjectMeta.ResourceVersion
				eventCh <- ev
			}
			resp.Body.Close()
		}
	}()

	return eventCh, errCh
}

func pollEvent(decoder *json.Decoder) (*Event, error) {
	re := &rawEvent{}
	err := decoder.Decode(re)
	if err != nil {
		if err == io.EOF {
			return nil, err
		}
		log.Errorf("fail to decode raw event from apiserver: %v", err)
		return nil, err
	}
	if re.Type == "ERROR" {
		log.Fatalf("watch error from apiserver. (TODO: separate transient and fatal error)\n"+
			"Error message: %s", re.Object)
	}
	ev := &Event{
		Type:   re.Type,
		Object: &spec.EtcdCluster{},
	}
	err = json.Unmarshal(re.Object, ev.Object)
	if err != nil {
		log.Errorf("fail to unmarshal EtcdCluster object from data (%s): %v", re.Object, err)
		return nil, err
	}
	return ev, nil
}
