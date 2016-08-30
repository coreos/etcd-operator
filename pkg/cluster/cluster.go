package cluster

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/kube-etcd-controller/pkg/util/etcdutil"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	"golang.org/x/net/context"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

type clusterEventType string

const (
	eventNewCluster    clusterEventType = "Add"
	eventDeleteCluster clusterEventType = "Delete"
	eventReconcile     clusterEventType = "Reconcile"
	eventModifyCluster clusterEventType = "Modify"
)

type clusterEvent struct {
	typ  clusterEventType
	spec Spec
	// currently running pods in kubernetes
	running etcdutil.MemberSet
}

type Cluster struct {
	kclient *unversioned.Client

	spec *Spec

	name string

	idCounter int
	eventCh   chan *clusterEvent
	stopCh    chan struct{}

	// members repsersents the members in the etcd cluster.
	// the name of the member is the the name of the pod the member
	// process runs in.
	members etcdutil.MemberSet

	backupDir string
}

func New(kclient *unversioned.Client, name string, spec *Spec) *Cluster {
	c := &Cluster{
		kclient: kclient,
		name:    name,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
		spec:    spec,
	}
	go c.run()

	return c
}

func (c *Cluster) Init(spec *Spec) {
	c.send(&clusterEvent{
		typ:  eventNewCluster,
		spec: *spec,
	})
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
	go c.monitorPods()
	for {
		select {
		case event := <-c.eventCh:
			switch event.typ {
			case eventNewCluster:
				c.create(&event.spec)
			case eventReconcile:
				if err := c.reconcile(event.running); err != nil {
					panic(err)
				}
			case eventModifyCluster:
				log.Printf("update: from: %#v, to: %#v", c.spec, event.spec)
				c.spec.Size = event.spec.Size
			case eventDeleteCluster:
				c.delete()
				close(c.stopCh)
				return
			}
		}
	}
}

func (c *Cluster) create(spec *Spec) {
	members := etcdutil.MemberSet{}
	c.spec = spec
	// we want to make use of member's utility methods.
	etcdName := fmt.Sprintf("%s-%04d", c.name, 0)
	members.Add(&etcdutil.Member{Name: etcdName})
	if err := c.createPodAndService(members, members[etcdName], "new"); err != nil {
		panic(fmt.Sprintf("(TODO: we need to clean up already created ones.)\nError: %v", err))
	}
	c.idCounter++
	log.Println("created cluster:", members)
}

func (c *Cluster) Update(spec *Spec) {
	// Only handles size change now. TODO: handle other updates.
	if spec.Size == c.spec.Size {
		return
	}
	c.send(&clusterEvent{
		typ:  eventModifyCluster,
		spec: *spec,
	})
}

func (c *Cluster) updateMembers(etcdcli *clientv3.Client) {
	resp, err := etcdcli.MemberList(context.TODO())
	if err != nil {
		panic(err)
	}
	c.members = etcdutil.MemberSet{}
	for _, m := range resp.Members {
		id := findID(m.Name)
		if id+1 > c.idCounter {
			c.idCounter = id + 1
		}

		c.members[m.Name] = &etcdutil.Member{
			Name: m.Name,
			ID:   m.ID,
		}
	}
}

func findID(name string) int {
	i := strings.LastIndex(name, "-")
	id, err := strconv.Atoi(name[i+1:])
	if err != nil {
		panic(err)
	}
	return id
}

func (c *Cluster) delete() {
	option := k8sapi.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": c.name,
		}),
	}

	pods, err := c.kclient.Pods("default").List(option)
	if err != nil {
		panic(err)
	}
	for i := range pods.Items {
		if err := c.removePodAndService(pods.Items[i].Name); err != nil {
			panic(err)
		}
	}
}

// todo: use a struct to replace the huge arg list.
func (c *Cluster) createPodAndService(members etcdutil.MemberSet, m *etcdutil.Member, state string) error {
	if err := k8sutil.CreateEtcdService(c.kclient, m.Name, c.name); err != nil {
		return err
	}
	return k8sutil.CreateEtcdPod(c.kclient, members.PeerURLPairs(), m, c.name, state, c.spec.AntiAffinity)
}

func (c *Cluster) removePodAndService(name string) error {
	err := c.kclient.Pods("default").Delete(name, nil)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	err = c.kclient.Services("default").Delete(name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
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
		log.Printf("saving snapshot from cluster %s error: %v", c.name, err)
		return err
	}

	err = os.Rename(tmpfile.Name(), nextSnapshotName)
	if err != nil {
		os.Remove(tmpfile.Name())
		log.Printf("renaming snapshot from cluster %s error: %v", c.name, err)
		return err
	}

	log.Printf("saved snapshot %v (size: %d) from cluster %s", n, nextSnapshotName, c.name)

	return nil
}

func (c *Cluster) monitorPods() {
	opts := k8sapi.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app":          "etcd",
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
		running := etcdutil.MemberSet{}
		for i := range podList.Items {
			running.Add(&etcdutil.Member{Name: podList.Items[i].Name})
		}

		c.send(&clusterEvent{
			typ:     eventReconcile,
			running: running,
		})
	}
}
