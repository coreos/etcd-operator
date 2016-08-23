package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/coreos/etcd/clientv3"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

type EtcdCluster struct {
	Kind       string            `json:"kind"`
	ApiVersion string            `json:"apiVersion"`
	Metadata   map[string]string `json:"metadata"`
	Spec       Spec              `json: "spec"`
}

type clusterEventType string

const (
	eventNewCluster    clusterEventType = "Add"
	eventDeleteCluster clusterEventType = "Delete"
	eventReconcile     clusterEventType = "Reconcile"
)

type clusterEvent struct {
	typ          clusterEventType
	size         int
	antiAffinity bool
	running      MemberSet
}

type Cluster struct {
	kclient *unversioned.Client

	antiAffinity bool

	name string

	// members repsersents the members in the etcd cluster.
	// the name of the member is the the name of the pod the member
	// process runs in.
	members MemberSet

	idCounter int
	eventCh   chan *clusterEvent
	stopCh    chan struct{}

	backupDir string
}

func newCluster(kclient *unversioned.Client, name string, spec Spec) *Cluster {
	c := &Cluster{
		kclient: kclient,
		name:    name,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
		members: MemberSet{},
	}
	go c.run()
	c.send(&clusterEvent{
		typ:          eventNewCluster,
		size:         spec.Size,
		antiAffinity: spec.AntiAffinity,
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
				c.create(event.size, event.antiAffinity)
			case eventReconcile:
				if err := c.reconcile(event.running); err != nil {
					panic(err)
				}
			case eventDeleteCluster:
				c.delete()
				close(c.stopCh)
				return
			}
		}
	}
}

func (c *Cluster) create(size int, antiAffinity bool) {
	c.antiAffinity = antiAffinity

	// we need this first in order to use PeerURLPairs as initialCluster parameter for etcd server.
	for i := 0; i < size; i++ {
		etcdName := fmt.Sprintf("%s-%04d", c.name, i)
		c.members.Add(&Member{Name: etcdName})
	}

	// TODO: parallelize it
	for i := 0; i < size; i++ {
		etcdName := fmt.Sprintf("%s-%04d", c.name, i)
		if err := c.createPodAndService(c.members[etcdName], "new"); err != nil {
			panic(fmt.Sprintf("(TODO: we need to clean up already created ones.)\nError: %v", err))
		}
		c.idCounter++
	}

	// hacky ordering to update initial members' IDs
	cfg := clientv3.Config{
		Endpoints:   c.members.ClientURLs(),
		DialTimeout: 5 * time.Second,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		panic(err)
	}
	err = waitMemberReady(etcdcli)
	if err != nil {
		panic(err)
	}
	resp, err := etcdcli.MemberList(context.TODO())
	if err != nil {
		panic(err)
	}
	for _, m := range resp.Members {
		c.members[m.Name].ID = m.ID
	}
	fmt.Println("created cluster:", c.members)
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
		c.removePodAndService(pods.Items[i].Name)
	}
}

// todo: use a struct to replace the huge arg list.
func (c *Cluster) createPodAndService(m *Member, state string) error {
	if err := createEtcdService(c.kclient, m.Name, c.name); err != nil {
		return err
	}
	return createEtcdPod(c.kclient, c.members.PeerURLPairs(), m, c.name, state, c.antiAffinity)
}

func (c *Cluster) removePodAndService(name string) error {
	err := c.kclient.Pods("default").Delete(name, nil)
	if err != nil {
		return err
	}
	err = c.kclient.Services("default").Delete(name)
	if err != nil {
		log.Printf("failed removing service (%s): %v", name, err)
	}
	return nil
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
		running := MemberSet{}
		for i := range podList.Items {
			running.Add(&Member{Name: podList.Items[i].Name})
		}
		c.send(&clusterEvent{
			typ:     eventReconcile,
			running: running,
		})
	}
}
