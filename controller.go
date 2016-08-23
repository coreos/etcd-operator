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

type Event struct {
	Type   string
	Object EtcdCluster
}

type etcdClusterController struct {
	kclient  *unversioned.Client
	clusters map[string]*Cluster
}

func (c *etcdClusterController) Run() {
	if err := c.createTPR(); err != nil {
		panic(err)
	}
	log.Println("etcd cluster controller starts running...")

	eventCh, errCh := monitorEtcdCluster(c.kclient.RESTClient.Client)
	for {
		select {
		case event := <-eventCh:
			clusterName := event.Object.Metadata["name"]
			switch event.Type {
			case "ADDED":
				c.clusters[clusterName] = newCluster(c.kclient, clusterName, event.Object.Spec)
			case "DELETED":
				c.clusters[clusterName].Delete()
				delete(c.clusters, clusterName)
			}
		case err := <-errCh:
			panic(err)
		}
	}
}

func (c *etcdClusterController) createTPR() error {
	tpr := &extensions.ThirdPartyResource{
		ObjectMeta: api.ObjectMeta{
			Name: "etcd-cluster.coreos.com",
		},
		Versions: []extensions.APIVersion{
			{Name: "v1"},
		},
		Description: "Managed etcd clusters",
	}
	_, err := c.kclient.ThirdPartyResources().Create(tpr)
	if err != nil {
		if isKubernetesResourceAlreadyExistError(err) {
			return nil
		}
		return err
	}

	err = wait.Poll(5*time.Second, 100*time.Second,
		func() (done bool, err error) {
			resp, err := watchETCDCluster(c.kclient.RESTClient.Client)
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

func watchETCDCluster(httpClient *http.Client) (*http.Response, error) {
	return httpClient.Get(masterHost + "/apis/coreos.com/v1/namespaces/default/etcdclusters?watch=true")
}

func monitorEtcdCluster(httpClient *http.Client) (<-chan *Event, <-chan error) {
	events := make(chan *Event)
	errc := make(chan error, 1)
	go func() {
		resp, err := watchETCDCluster(httpClient)
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
