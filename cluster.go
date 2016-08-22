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

type clusterEventType string

const (
	eventNewCluster    clusterEventType = "Add"
	eventDeleteCluster clusterEventType = "Delete"
	eventModifyCluster clusterEventType = "Modify"
	eventReconcile     clusterEventType = "Reconcile"
)

type clusterEvent struct {
	typ          clusterEventType
	size         int
	antiAffinity bool
}

type Cluster struct {
	kclient *unversioned.Client

	antiAffinity bool
	size         int

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
		members: make(MemberSet),
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
				if err := c.reconcile(); err != nil {
					panic(fmt.Errorf("error reconciling cluster state: %v", err))
				}
			case eventModifyCluster:
				fmt.Printf("Modify ev detected\n")
				c.size = event.size
				//TODO: when we're ready
				//c.antiAffinity = event.antiAffinity
				//... will probably want to rename reconcileClusterSize() to reconcileClusterSpec()
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
	c.size = size
}

//Both Inc/Dec node count do no locking, must be dispatched from event handler

//TODO: make transactional
func (c *Cluster) incrementNodeCount() error {
	fmt.Printf(" > increment node count: %d / %d\n", len(c.members), c.size-1)
	etcdName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	newMember := &Member{Name: etcdName}

	if len(c.members) == 0 {
		c.members.Add(newMember)

		if err := c.createPodAndService(newMember, "new"); err != nil {
			return err
		}
		clustercli, err := getEtcdClusterClient(c.members.ClientURLs(), 60)
		if err != nil {
			return err
		}

		resp, err := clustercli.MemberList(context.TODO())
		if err != nil {
			return err
		}

		newMember.ID = resp.Members[0].ID
	} else {
		clustercli, err := getEtcdClusterClient(c.members.ClientURLs(), 15)
		if err != nil {
			return err
		}

		c.members.Add(newMember)

		if err := c.createPodAndService(newMember, "existing"); err != nil {
			return err
		}
		resp, err := clustercli.MemberAdd(context.TODO(), []string{newMember.PeerAddr()})
		if err != nil {
			return err
		}
		newMember.ID = resp.Member.ID
	}

	c.idCounter++
	return nil
}

//TODO: make transactional
func (c *Cluster) decrementNodeCount() error {
	fmt.Printf(" < decrement node count: %d / %d\n", len(c.members), c.size-1)
	m := c.members.PickOne()
	c.members.Remove(m.Name)

	clustercli, err := getEtcdClusterClient(c.members.ClientURLs(), 5)
	if err != nil {
		return err
	}
	if _, err := clustercli.MemberRemove(context.TODO(), m.ID); err != nil {
		return err
	}
	if err := c.removePodAndService(m.Name); err != nil {
		return err
	}

	return nil
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
		return err
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
	// TODO: Select "etcd_node" to remove left service.
	for {
		select {
		case <-c.stopCh:
			return
		case <-time.After(5 * time.Second):
		}

		c.send(&clusterEvent{
			typ: eventReconcile,
		})
	}
}

func (c *Cluster) getRunningEtcdPods() MemberSet {
	opts := api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": c.name,
		}),
	}

	podList, err := c.kclient.Pods("default").List(opts)
	if err != nil {
		panic(err)
	}
	running := MemberSet{}
	for i := range podList.Items {
		running.Add(&Member{Name: podList.Items[i].Name})
	}
	return running
}
