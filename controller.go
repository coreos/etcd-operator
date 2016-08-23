package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/util/wait"
)

const (
	tprName = "etcd-cluster.coreos.com"
)

type Event struct {
	Type   string
	Object EtcdCluster
}

type etcdClusterController struct {
	kclient  *unversioned.Client
	clusters map[string]*Cluster
}

func (c *etcdClusterController) Run() {
	watchVersion := "0"
	if err := c.createTPR(); err != nil {
		switch {
		case isKubernetesResourceAlreadyExistError(err):
			watchVersion, err = c.findAllClusters()
			if err != nil {
				panic(err)
			}
		default:
			panic(err)
		}
	}
	log.Println("etcd cluster controller starts running...")

	eventCh, errCh := monitorEtcdCluster(c.kclient.RESTClient.Client, watchVersion)
	for {
		select {
		case event := <-eventCh:
			clusterName := event.Object.ObjectMeta.Name
			switch event.Type {
			case "ADDED":
				nc := newCluster(c.kclient, clusterName)
				nc.init(event.Object.Spec)
				c.clusters[clusterName] = nc
			case "DELETED":
				c.clusters[clusterName].Delete()
				delete(c.clusters, clusterName)
			}
		case err := <-errCh:
			panic(err)
		}
	}
}

func (c *etcdClusterController) findAllClusters() (string, error) {
	log.Println("finding existing clusters...")
	resp, err := listETCDCluster(c.kclient.RESTClient.Client)
	if err != nil {
		return "", err
	}
	d := json.NewDecoder(resp.Body)
	list := &EtcdClusterList{}
	if err := d.Decode(list); err != nil {
		return "", err
	}
	for _, item := range list.Items {
		nc := newCluster(c.kclient, item.Name)
		c.clusters[item.Name] = nc
	}
	return list.ListMeta.ResourceVersion, nil
}

func (c *etcdClusterController) createTPR() error {
	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: api.ObjectMeta{
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

	err = wait.Poll(5*time.Second, 100*time.Second,
		func() (done bool, err error) {
			resp, err := watchETCDCluster(c.kclient.RESTClient.Client, "0")
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

func monitorEtcdCluster(httpClient *http.Client, watchVersion string) (<-chan *Event, <-chan error) {
	events := make(chan *Event)
	errc := make(chan error, 1)
	go func() {
		resp, err := watchETCDCluster(httpClient, watchVersion)
		if err != nil {
			errc <- err
			return
		}
		if resp.StatusCode != 200 {
			errc <- errors.New("Invalid status code: " + resp.Status)
			return
		}
		log.Println("start watching...")
		for {
			decoder := json.NewDecoder(resp.Body)
			ev := new(Event)
			err = decoder.Decode(ev)
			if err != nil {
				errc <- err
			}
			log.Printf("etcd cluster event: %v %#v\n", ev.Type, ev.Object)
			events <- ev
		}
	}()

	return events, errc
}
