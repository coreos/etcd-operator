package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/pkg/cluster"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

const (
	tprName = "etcd-cluster.coreos.com"
)

var (
	supportedPVProvisioners = map[string]struct{}{
		"kubernetes.io/gce-pd":  {},
		"kubernetes.io/aws-ebs": {},
	}
)

type Event struct {
	Type   string
	Object cluster.EtcdCluster
}

type Controller struct {
	*Config
	kclient  *unversioned.Client
	clusters map[string]*cluster.Cluster
}

type Config struct {
	Namespace     string
	MasterHost    string
	TLSInsecure   bool
	TLSConfig     restclient.TLSClientConfig
	PVProvisioner string
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

func New(cfg *Config) *Controller {
	if err := cfg.validate(); err != nil {
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
	}
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
				nc := cluster.New(c.kclient, clusterName, c.Namespace, clusterSpec)
				c.clusters[clusterName] = nc

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
		nc := cluster.Restore(c.kclient, item.Name, c.Namespace, &item.Spec)
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

	return k8sutil.WaitEtcdTPRReady(c.kclient.Client, 3*time.Second, 90*time.Second, c.MasterHost, c.Namespace)
}

func monitorEtcdCluster(host, ns string, httpClient *http.Client, watchVersion string) (<-chan *Event, <-chan error) {
	events := make(chan *Event)
	// On unexpected error case, controller should exit
	errc := make(chan error, 1)
	go func() {
		for {
			resp, err := k8sutil.WatchETCDCluster(host, ns, httpClient, watchVersion)
			if err != nil {
				errc <- err
				return
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				errc <- errors.New("Invalid status code: " + resp.Status)
				return
			}
			log.Printf("watching at %v", watchVersion)
			for {
				decoder := json.NewDecoder(resp.Body)
				ev := new(Event)
				err = decoder.Decode(ev)
				if err != nil {
					if err == io.EOF {
						break
					}
					log.Errorf("failed to get event from apiserver: %v", err)
					errc <- err
					return
				}
				if ev.Type == "ERROR" {
					// TODO: We couldn't decode error status from watch stream on apiserver.
					//       Working around by restart and go through recover path.
					//       We strive to fix it in k8s upstream.
					log.Fatal("unkown watch error from apiserver")
				}
				log.Printf("etcd cluster event: %v %#v", ev.Type, ev.Object)
				watchVersion = ev.Object.ObjectMeta.ResourceVersion
				events <- ev
			}
			resp.Body.Close()
		}
	}()

	return events, errc
}
