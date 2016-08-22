package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

var (
	masterHost  string
	tlsInsecure bool
	tlsConfig   restclient.TLSClientConfig
)

func init() {
	flag.StringVar(&masterHost, "master", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flag.StringVar(&tlsConfig.CertFile, "cert-file", "", " - NOT RECOMMENDED FOR PRODUCTION - Path to public TLS certificate file.")
	flag.StringVar(&tlsConfig.KeyFile, "key-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to private TLS certificate file.")
	flag.StringVar(&tlsConfig.CAFile, "ca-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to TLS CA file.")
	flag.BoolVar(&tlsInsecure, "tls-insecure", false, "- NOT RECOMMENDED FOR PRODUCTION - Don't verify API server's CA certificate.")
	flag.Parse()
}

type EtcdCluster struct {
	Kind       string                 `json:"kind"`
	ApiVersion string                 `json:"apiVersion"`
	Metadata   map[string]interface{} `json:"metadata"`
	Spec       Spec                   `json: "spec"`
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
			fmt.Printf("EVENT %+v\n", event)
			var clusterName string
			var ok bool
			if clusterName, ok = event.Object.Metadata["name"].(string); !ok {
				panic("Could not cast metadata.name as a string")
			}
			switch event.Type {
			case "ADDED":
				c.clusters[clusterName] = newCluster(c.kclient, clusterName, event.Object.Spec)
			case "DELETED":
				c.clusters[clusterName].Delete()
				delete(c.clusters, clusterName)
			case "MODIFIED":
				c.clusters[clusterName].send(&clusterEvent{
					typ:          eventModifyCluster,
					size:         event.Object.Spec.Size,
					antiAffinity: event.Object.Spec.AntiAffinity,
				})
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
				errc <- fmt.Errorf("error decoding event json: %v", err)
			}
			log.Printf("etcd cluster event: %v %#v\n", ev.Type, ev.Object)
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

	hostUrl, err := url.Parse(host)
	if err != nil {
		panic(fmt.Sprintf("error parsing host url %s : %v", host, err))
	}
	cfg := &restclient.Config{
		Host:  host,
		QPS:   100,
		Burst: 100,
	}
	if hostUrl.Scheme == "https" {
		cfg.TLSClientConfig = tlsConfig
		cfg.Insecure = tlsInsecure
	}
	c, err := unversioned.New(cfg)
	if err != nil {
		panic(err)
	}
	return c
}
