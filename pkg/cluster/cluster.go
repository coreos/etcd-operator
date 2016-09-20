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

func New(c *unversioned.Client, name string, spec *Spec) *Cluster {
	return new(c, name, spec, true)
}

func Restore(c *unversioned.Client, name string, spec *Spec) *Cluster {
	return new(c, name, spec, false)
}

func new(kclient *unversioned.Client, name string, spec *Spec, isNewCluster bool) *Cluster {
	c := &Cluster{
		kclient: kclient,
		name:    name,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
		spec:    spec,
	}
	if isNewCluster {
		if err := c.newSeedMember(); err != nil {
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
			// currently running pods in kubernetes mapped to member set
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
	return &etcdutil.Member{Name: etcdName, HostNetwork: c.spec.HostNetwork}
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

		// Determine the clusterIP for the Service for this pod.
		// TODO: We shouldn't have to query the API all the time to get this.
		// Let's just store the information somewhere.  Just hacked together for now.
		svc, err := c.kclient.Services("default").Get(m.Name)
		if err != nil {
			panic(err)
		}

		c.members[m.Name] = &etcdutil.Member{
			Name:        m.Name,
			ID:          m.ID,
			HostNetwork: c.spec.HostNetwork,
			ClusterIP:   svc.Spec.ClusterIP,
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

func (c *Cluster) createPodAndService(members etcdutil.MemberSet, m *etcdutil.Member, state string, needRecovery bool) error {
	svcIP, err := c.ensureService(m.Name)
	if err != nil {
		return err
	}
	m.ClusterIP = svcIP
	token := ""
	if state == "new" {
		token = uuid.New()
	}
	pod := k8sutil.MakeEtcdPod(m, members.PeerURLPairs(), c.name, state, token, c.spec.AntiAffinity, c.spec.HostNetwork)
	if needRecovery {
		k8sutil.AddRecoveryToPod(pod, c.name, m.Name, token)
	}
	return k8sutil.CreateAndWaitPod(c.kclient, pod, m)
}

// ensureService ensures that the given Service exists, and returns
// its ClusterIP.
func (c *Cluster) ensureService(memberName string) (string, error) {
	// See if the desired Service exists.
	svc, err := c.kclient.Services("default").Get(memberName)
	if err != nil {
		// Service did not exist - create it.
		svcIP, createErr := k8sutil.CreateEtcdService(c.kclient, memberName, c.name)
		if createErr != nil {
			// Failed to create - return error.
			return "", err
		}
		return svcIP, nil
	}
	return svc.Spec.ClusterIP, nil
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

func (c *Cluster) getRunning() (etcdutil.MemberSet, error) {
	opts := k8sapi.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app":          "etcd",
			"etcd_cluster": c.name,
		}),
	}

	podList, err := c.kclient.Pods("default").List(opts)
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

		// Determine the clusterIP for the Service for this pod.
		// TODO: This is only used for hostNetwork: true - should we limit?
		svc, err := c.kclient.Services("default").Get(pod.Name)
		if err != nil {
			return nil, fmt.Errorf("Failed to get svc for pod %s", pod.Name)
		}

		running.Add(&etcdutil.Member{
			Name:        pod.Name,
			HostNetwork: c.spec.HostNetwork,
			ClusterIP:   svc.Spec.ClusterIP,
		})
	}
	return running, nil
}
