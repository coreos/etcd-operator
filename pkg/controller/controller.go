package controller

import (
	"encoding/json"
	"errors"
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
	"k8s.io/kubernetes/pkg/util/wait"
)

const (
	tprName = "etcd-cluster.coreos.com"
)

type Event struct {
	Type   string
	Object cluster.EtcdCluster
}

type Controller struct {
	masterHost string
	kclient    *unversioned.Client
	clusters   map[string]*cluster.Cluster
}

type Config struct {
	MasterHost  string
	TLSInsecure bool
	TLSConfig   restclient.TLSClientConfig
}

func New(cfg Config) *Controller {
	host, c := getClient(cfg)
	return &Controller{
		masterHost: host,
		kclient:    c,
		clusters:   make(map[string]*cluster.Cluster),
	}
}

func getClient(cfg Config) (string, *unversioned.Client) {
	if len(cfg.MasterHost) == 0 {
		inCfg, err := restclient.InClusterConfig()
		if err != nil {
			panic(err)
		}
		client, err := unversioned.NewInCluster()
		if err != nil {
			panic(err)
		}
		return inCfg.Host, client
	}
	return cfg.MasterHost, k8sutil.MustCreateClient(cfg.MasterHost, cfg.TLSInsecure, cfg.TLSConfig)
}

func (c *Controller) Run() {
	watchVersion := "0"
	if err := c.createTPR(); err != nil {
		switch {
		case k8sutil.IsKubernetesResourceAlreadyExistError(err):
			watchVersion, err = c.findAllClusters()
			if err != nil {
				panic(err)
			}
		default:
			panic(err)
		}
	}
	log.Println("etcd cluster controller starts running...")

	eventCh, errCh := monitorEtcdCluster(c.masterHost, c.kclient.RESTClient.Client, watchVersion)
	for {
		select {
		case event := <-eventCh:
			clusterName := event.Object.ObjectMeta.Name
			switch event.Type {
			case "ADDED":
				nc := cluster.New(c.kclient, clusterName, &event.Object.Spec)
				// TODO: combine init into New. Different fresh new and recovered new.
				nc.Init(&event.Object.Spec)
				c.clusters[clusterName] = nc
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
	resp, err := k8sutil.ListETCDCluster(c.masterHost, c.kclient.RESTClient.Client)
	if err != nil {
		return "", err
	}
	d := json.NewDecoder(resp.Body)
	list := &EtcdClusterList{}
	if err := d.Decode(list); err != nil {
		return "", err
	}
	for _, item := range list.Items {
		nc := cluster.New(c.kclient, item.Name, &item.Spec)
		c.clusters[item.Name] = nc
	}
	return list.ListMeta.ResourceVersion, nil
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

	err = wait.Poll(3*time.Second, 100*time.Second,
		func() (done bool, err error) {
			resp, err := k8sutil.WatchETCDCluster(c.masterHost, c.kclient.RESTClient.Client, "0")
			if err != nil {
				return false, err
			}
			if resp.StatusCode == 200 {
				return true, nil
			}
			if resp.StatusCode == 404 {
				return false, nil
			}
			return false, errors.New("Invalid status code: " + resp.Status)
		})
	return err
}

func monitorEtcdCluster(host string, httpClient *http.Client, watchVersion string) (<-chan *Event, <-chan error) {
	events := make(chan *Event)
	errc := make(chan error, 1)
	go func() {
		for {
			resp, err := k8sutil.WatchETCDCluster(host, httpClient, watchVersion)
			if err != nil {
				errc <- err
				return
			}
			if resp.StatusCode != 200 {
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
					errc <- err
				}
				log.Printf("etcd cluster event: %v %#v", ev.Type, ev.Object)
				// There might be cases of transient errors, e.g. network disconnection.
				// We can backoff and rewatch in such cases.
				if ev.Type == "ERROR" {
					break
				}
				watchVersion = ev.Object.ObjectMeta.ResourceVersion
				events <- ev
			}
		}
	}()

	return events, errc
}
