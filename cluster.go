package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"time"

	"golang.org/x/net/context"

	"github.com/coreos/etcd/clientv3"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

type clusterEventType string

const (
	eventNewCluster    clusterEventType = "Add"
	eventDeleteCluster clusterEventType = "Delete"
)

type clusterEvent struct {
	typ  clusterEventType
	size int
}

type Cluster struct {
	kclient   *unversioned.Client
	name      string
	idCounter int
	eventCh   chan *clusterEvent
	stopCh    chan struct{}

	backupDir string
}

func newCluster(kclient *unversioned.Client, name string, size int) *Cluster {
	c := &Cluster{
		kclient: kclient,
		name:    name,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
	}
	go c.run()
	c.send(&clusterEvent{
		typ:  eventNewCluster,
		size: size,
	})
	return c
}

func (c *Cluster) Delete() {
	c.send(&clusterEvent{typ: eventDeleteCluster})
}

func (c *Cluster) send(ev *clusterEvent) {
	select {
	case c.eventCh <- ev:
	case <-c.stopCh:
	default:
		panic("TODO: too many events queued...")
	}
}

func (c *Cluster) run() {
	go c.monitorMembers()

	for {
		select {
		case event := <-c.eventCh:
			switch event.typ {
			case eventNewCluster:
				c.create(event.size)
			case eventDeleteCluster:
				c.delete()
				close(c.stopCh)
				return
			}
		}
	}
}

func (c *Cluster) create(size int) {
	initialCluster := []string{}
	for i := 0; i < size; i++ {
		etcdName := fmt.Sprintf("%s-%04d", c.name, i)
		initialCluster = append(initialCluster, fmt.Sprintf("%s=%s", etcdName, makeEtcdPeerAddr(etcdName)))
	}

	for i := 0; i < size; i++ {
		if err := c.launchNewMember(c.idCounter, initialCluster, "new"); err != nil {
			// TODO: we need to clean up already created ones.
			panic(err)
		}
		c.idCounter++
	}
}

func (c *Cluster) launchNewMember(id int, initialCluster []string, state string) error {
	etcdName := fmt.Sprintf("%s-%04d", c.name, id)
	if err := createEtcdService(c.kclient, etcdName, c.name); err != nil {
		return err
	}
	return createEtcdPod(c.kclient, etcdName, c.name, initialCluster, state)
}

func (c *Cluster) delete() {
	option := api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": c.name,
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

func (c *Cluster) backup() error {
	clientAddr := "todo"
	nextSnapshotName := "todo"

	cfg := clientv3.Config{
		Endpoints: []string{clientAddr},
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	log.Println("saving snapshot from cluster", c.name)

	rc, err := etcdcli.Maintenance.Snapshot(ctx)
	cancel()
	if err != nil {
		return err
	}

	tmpfile, err := ioutil.TempFile(c.backupDir, "snapshot")
	n, err := io.Copy(tmpfile, rc)
	if err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		log.Printf("saving snapshot from cluster %s error: %v\n", c.name, err)
		return err
	}

	err = os.Rename(tmpfile.Name(), nextSnapshotName)
	if err != nil {
		os.Remove(tmpfile.Name())
		log.Printf("renaming snapshot from cluster %s error: %v\n", c.name, err)
		return err
	}

	log.Printf("saved snapshot %v (size: %d) from cluster %s", n, nextSnapshotName, c.name)

	return nil
}

func (c *Cluster) monitorMembers() {
	opts := api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": c.name,
		}),
	}
	// TODO: Select "etcd_node" to remove left service.
	for {
		select {
		case <-c.stopCh:
			return
		case <-time.After(5 * time.Second):
		}

		podList, err := c.kclient.Pods("default").List(opts)
		if err != nil {
			panic(err)
		}
		P := MemberSet{}
		for i := range podList.Items {
			P = append(P, Member{Name: podList.Items[i].Name})
		}

		if P.Size() == 0 {
			panic("TODO: All pods removed. Impossible. Anyway, we can't create etcd client.")
		}

		// TODO: put this into central event handling
		cfg := clientv3.Config{
			Endpoints: []string{makeClientAddr(P[0].Name)},
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			panic(err)
		}
		resp, err := etcdcli.MemberList(context.TODO())
		if err != nil {
			panic(err)
		}

		M := MemberSet{}
		for _, member := range resp.Members {
			M = append(M, Member{
				Name: member.Name,
				ID:   member.ID,
			})
		}

		if err := c.reconcile(P, M); err != nil {
			panic(err)
		}
	}
}
