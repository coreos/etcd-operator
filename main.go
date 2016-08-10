package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"

	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

var masterHost string

func init() {
	flag.StringVar(&masterHost, "master", "", "TODO: usage")
	flag.Parse()
}

type EtcdCluster struct {
	Kind       string            `json:"kind"`
	ApiVersion string            `json:"apiVersion"`
	Metadata   map[string]string `json:"metadata"`
	Size       int               `json:"size"`
}

type Event struct {
	Type   string
	Object EtcdCluster
}

type etcdClusterController struct {
	kclient  *unversioned.Client
	clusters map[string]*Cluster
}

func (c *etcdClusterController) Run() {
	eventCh, errCh := monitorEtcdCluster(c.kclient.RESTClient.Client)
	for {
		select {
		case event := <-eventCh:
			clusterName := event.Object.Metadata["name"]
			switch event.Type {
			case "ADDED":
				clus := newCluster(c.kclient, clusterName)
				c.clusters[clusterName] = clus
				go clus.Run()
				clus.Handle(event)
			case "DELETED":
				clus := c.clusters[clusterName]
				clus.Handle(event)
				clus.Stop()
				delete(c.clusters, clusterName)
			}
		case err := <-errCh:
			panic(err)
		}
	}
}

func monitorEtcdCluster(httpClient *http.Client) (<-chan *Event, <-chan error) {
	events := make(chan *Event)
	errc := make(chan error, 1)
	go func() {
		resp, err := httpClient.Get(masterHost + "/apis/coreos.com/v1/namespaces/default/etcdclusters?watch=true")
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
			log.Println("etcd cluster event:", ev.Type, ev.Object.Size, ev.Object.Metadata)
			events <- ev
		}
	}()

	return events, errc
}

func main() {
	c := &etcdClusterController{
		kclient:  mustCreateClient(masterHost),
		clusters: make(map[string]*Cluster),
	}
	log.Println("etcd cluster controller starts running...")
	c.Run()
}

func mustCreateClient(host string) *unversioned.Client {
	if len(host) == 0 {
		cfg, err := restclient.InClusterConfig()
		if err != nil {
			panic(err)
		}
		c, err := unversioned.NewInCluster()
		if err != nil {
			panic(err)
		}
		masterHost = cfg.Host
		return c
	}

	cfg := &restclient.Config{
		Host:  host,
		QPS:   100,
		Burst: 100,
	}
	c, err := unversioned.New(cfg)
	if err != nil {
		panic(err)
	}
	return c
}
