package main

import (
	"fmt"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

type Cluster struct {
	kclient *unversioned.Client
	eventCh chan *Event
	stopCh  chan struct{}
}

func newCluster(kclient *unversioned.Client) *Cluster {
	return &Cluster{
		kclient: kclient,
		eventCh: make(chan *Event, 100),
		stopCh:  make(chan struct{}),
	}
}

func (c *Cluster) Run() {
	for {
		select {
		case event := <-c.eventCh:
			switch event.Type {
			case "ADDED":
				c.createCluster(event.Object)
			case "DELETED":
				c.deleteCluster(event.Object)
			}
		case <-c.stopCh:
		}
	}
}

func (c *Cluster) Stop() {
	close(c.stopCh)
}

func (c *Cluster) Handle(ev *Event) {
	select {
	case c.eventCh <- ev:
	case <-c.stopCh:
	default:
		panic("TODO: too many events queued...")
	}
}

func (c *Cluster) createCluster(cluster EtcdCluster) {
	size := cluster.Size
	clusterName := cluster.Metadata["name"]

	initialCluster := []string{}
	for i := 0; i < size; i++ {
		initialCluster = append(initialCluster, fmt.Sprintf("%s-%04d=http://%s-%04d:2380", clusterName, i, clusterName, i))
	}

	for i := 0; i < size; i++ {
		etcdName := fmt.Sprintf("%s-%04d", clusterName, i)

		svc := makeEtcdService(etcdName, clusterName)
		_, err := c.kclient.Services("default").Create(svc)
		if err != nil {
			panic(err)
		}
		// TODO: add and expose client port
		pod := makeEtcdPod(etcdName, clusterName, initialCluster)
		_, err = c.kclient.Pods("default").Create(pod)
		if err != nil {
			panic(err)
		}
	}
}

func (c *Cluster) deleteCluster(cluster EtcdCluster) {
	clusterName := cluster.Metadata["name"]
	option := api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": clusterName,
		}),
	}

	pods, err := c.kclient.Pods("default").List(option)
	if err != nil {
		panic(err)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		err = c.kclient.Pods("default").Delete(pod.Name, nil)
		if err != nil {
			panic(err)
		}
	}

	services, err := c.kclient.Services("default").List(option)
	if err != nil {
		panic(err)
	}
	for i := range services.Items {
		service := &services.Items[i]
		err = c.kclient.Services("default").Delete(service.Name)
		if err != nil {
			panic(err)
		}
	}
}
