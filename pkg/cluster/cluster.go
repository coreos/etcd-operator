package cluster

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/kube-etcd-controller/pkg/util/constants"
	"github.com/coreos/kube-etcd-controller/pkg/util/etcdutil"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
	"github.com/pborman/uuid"
	"golang.org/x/net/context"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

type clusterEventType string

const (
	eventDeleteCluster clusterEventType = "Delete"
	eventModifyCluster clusterEventType = "Modify"
)

type clusterEvent struct {
	typ  clusterEventType
	spec Spec
}

type Cluster struct {
	kclient *unversioned.Client

	spec *Spec

	name      string
	namespace string

	idCounter int
	eventCh   chan *clusterEvent
	stopCh    chan struct{}

	// members repsersents the members in the etcd cluster.
	// the name of the member is the the name of the pod the member
	// process runs in.
	members etcdutil.MemberSet

	backupDir string
}

func New(c *unversioned.Client, name, ns string, spec *Spec) *Cluster {
	return new(c, name, ns, spec, true)
}

func Restore(c *unversioned.Client, name, ns string, spec *Spec) *Cluster {
	return new(c, name, ns, spec, false)
}

func new(kclient *unversioned.Client, name, ns string, spec *Spec, isNewCluster bool) *Cluster {
	c := &Cluster{
		kclient:   kclient,
		name:      name,
		namespace: ns,
		eventCh:   make(chan *clusterEvent, 100),
		stopCh:    make(chan struct{}),
		spec:      spec,
	}
	if isNewCluster {
		var err error
		if spec.Seed == nil {
			err = c.newSeedMember()
		} else {
			err = c.migrateSeedMember()
		}
		if err != nil {
			panic(err)
		}
	}
	go c.run()

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
	for {
		select {
		case event := <-c.eventCh:
			switch event.typ {
			case eventModifyCluster:
				log.Printf("update: from: %#v, to: %#v", c.spec, event.spec)
				c.spec.Size = event.spec.Size
			case eventDeleteCluster:
				c.delete()
				close(c.stopCh)
				return
			}
		case <-time.After(5 * time.Second):
			// currently running pods in kubernets mapped to member set
			running, err := c.getRunning()
			if err != nil {
				panic(err)
			}
			if running.Size() == 0 {
				log.Infof("cluster (%s) not ready yet", c.name)
				continue
			}
			if err := c.reconcile(running); err != nil {
				panic(err)
			}
		}
	}
}

func (c *Cluster) makeSeedMember() *etcdutil.Member {
	etcdName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	return &etcdutil.Member{Name: etcdName}
}

func (c *Cluster) startSeedMember(recoverFromBackup bool) error {
	m := c.makeSeedMember()
	if err := c.createPodAndService(etcdutil.NewMemberSet(m), m, "new", recoverFromBackup); err != nil {
		return fmt.Errorf("failed to create seed member (%s): %v", m.Name, err)
	}
	c.idCounter++
	log.Infof("created cluster (%s) with seed member (%s)", c.name, m.Name)
	return nil
}

func (c *Cluster) newSeedMember() error {
	return c.startSeedMember(false)
}

func (c *Cluster) restoreSeedMember() error {
	return c.startSeedMember(true)
}

func (c *Cluster) migrateSeedMember() error {
	// add a new member into the existing seed cluster

	// wait for the delay
	delay := time.Duration(c.spec.Seed.RemoveDelay) * time.Second
	log.Infof("wait %v before remove the original seed member", delay)
	time.Sleep(delay)

	// delete the original seed member from the etcd cluster.
	// now we have migrate the seed member into kubernetes.
	// our controller not takes control over it.

	return nil
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
	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberList(ctx)
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

	pods, err := c.kclient.Pods(c.namespace).List(option)
	if err != nil {
		panic(err)
	}
	for i := range pods.Items {
		if err := c.removePodAndService(pods.Items[i].Name); err != nil {
			panic(err)
		}
	}
}

func (c *Cluster) createPodAndService(members etcdutil.MemberSet, m *etcdutil.Member, state string, needRecovery bool) error {
	if err := k8sutil.CreateEtcdService(c.kclient, m.Name, c.name, c.namespace); err != nil {
		return err
	}
	token := ""
	if state == "new" {
		token = uuid.New()
	}
	pod := k8sutil.MakeEtcdPod(m, members.PeerURLPairs(), c.name, state, token, c.spec.AntiAffinity, c.spec.HostNetwork)
	if needRecovery {
		k8sutil.AddRecoveryToPod(pod, c.name, m.Name, token)
	}
	return k8sutil.CreateAndWaitPod(c.kclient, pod, m, c.namespace)
}

func (c *Cluster) removePodAndService(name string) error {
	err := c.kclient.Pods(c.namespace).Delete(name, nil)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	err = c.kclient.Services(c.namespace).Delete(name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) getRunning() (etcdutil.MemberSet, error) {
	opts := k8sapi.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app":          "etcd",
			"etcd_cluster": c.name,
		}),
	}

	podList, err := c.kclient.Pods(c.namespace).List(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list running pods: %v", err)
	}
	running := etcdutil.MemberSet{}
	for i := range podList.Items {
		pod := podList.Items[i]
		// TODO: use liveness probe to do checking
		if pod.Status.Phase != k8sapi.PodRunning {
			log.Debugf("skipping non-running pod: %s", pod.Name)
			continue
		}
		running.Add(&etcdutil.Member{Name: pod.Name})
	}
	return running, nil
}
