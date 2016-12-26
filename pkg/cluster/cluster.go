package cluster

import (
	"fmt"
	"sync"
	"time"

	"github.com/GregoryIan/operator/pkg/cluster/member"
	"github.com/GregoryIan/operator/pkg/spec"
	"github.com/GregoryIan/operator/pkg/util"
	"github.com/juju/errors"
	"github.com/ngaut/log"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

type clusterEventType string

const (
	eventDeleteCluster clusterEventType = "Delete"
	eventModifyCluster clusterEventType = "Modify"
)

type Config struct {
	Name      string
	Namespace string
	KubeCli   *unversioned.Client
}

type clusterEvent struct {
	typ  clusterEventType
	spec spec.ClusterSpec
}

type Cluster struct {
	members map[member.MemberType]member.MemberSet

	Config

	spec    *spec.ClusterSpec
	eventCh chan *clusterEvent
	stopCh  chan struct{}
}

func New(c Config, s *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup) (*Cluster, error) {
	return new(c, s, stopC, wg, true)
}

func Restore(c Config, s *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup) (*Cluster, error) {
	return new(c, s, stopC, wg, false)
}

func new(config Config, s *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup, isNewCluster bool) (*Cluster, error) {
	c := &Cluster{
		Config:  config,
		spec:    s,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
	}

	if isNewCluster {
		if err := c.prepareSeedMember(); err != nil {
			return nil, errors.Trace(err)
		}
		if err := c.createClientServiceLB(); err != nil {
			return nil, errors.Errorf("fail to create client service LB: %v", err)
		}
	} else {
		for _, tp := range member.ServiceAdjustSequence {
			c.members[tp] = member.GetEmptyMemberSet(tp)
		}
	}
	member.SplitAndDistributeSpec(s, c.spec, c.members)

	go c.run(stopC, wg)
	return c, nil
}

func (c *Cluster) prepareSeedMember() error {
	members, err := member.InitSeedMembers(c.KubeCli, c.Name, c.Namespace, c.spec)
	if err != nil {
		return errors.Trace(err)
	}

	c.members = members
	return nil
}

func (c *Cluster) run(stopC <-chan struct{}, wg *sync.WaitGroup) {
	needDeleteCluster := true
	wg.Add(1)
	defer func() {
		if needDeleteCluster {
			log.Infof("deleting cluster")
			c.delete()
		}
		close(c.stopCh)
		wg.Done()
	}()

	for {
		select {
		case <-stopC:
			needDeleteCluster = false
			return
		case event := <-c.eventCh:
			switch event.typ {
			case eventModifyCluster:
				log.Infof("spec update: from: %v to: %v", c.spec, event.spec)
				member.SplitAndDistributeSpec(&event.spec, c.spec, c.members)
			case eventDeleteCluster:
				return
			}
		case <-time.After(5 * time.Second):
			if c.spec.Paused {
				log.Infof("control is paused, skipping reconcilation")
				continue
			}

			for _, memberTp := range member.ServiceAdjustSequence {
				// todo: go func
				// query the pods and its' state that in kubernetes
				running, pending, err := c.pollPods(memberTp)
				if err != nil {
					log.Errorf("fail to poll memberType: %v pods: %v", memberTp, err)
					continue
				}
				if len(pending) > 0 {
					log.Infof("skip reconciliation: memberType: %v running (%v), pending (%v)", memberTp, util.GetPodNames(running), util.GetPodNames(pending))
					continue
				}
				if len(running) == 0 {
					log.Fatalf("all memberType: %v pods are dead.", memberTp)
				}
				if err := c.reconcile(running, memberTp); err != nil {
					log.Errorf("fail to reconcile: %v", err)
				}
			}
		}
	}
}

func (c *Cluster) createClientServiceLB() error {
	if _, err := member.CreatePDService(c.KubeCli, fmt.Sprintf("%s-pd", c.Name), c.Namespace); err != nil {
		if !util.IsKubernetesResourceAlreadyExistError(err) {
			return errors.Trace(err)
		}
	}

	return nil
}

func (c *Cluster) Update(spec *spec.ClusterSpec) {
	c.send(&clusterEvent{typ: eventModifyCluster, spec: *spec})
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

func (c *Cluster) delete() {
	option := k8sapi.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"tidb_cluster": c.Name,
		}),
	}

	pods, err := c.KubeCli.Pods(c.Namespace).List(option)
	if err != nil {
		log.Errorf("cluster deletion: cannot delete any pod due to failure to list: %v", err)
	} else {
		for i := range pods.Items {
			if err := c.removePod(pods.Items[i].Name); err != nil {
				log.Errorf("cluster deletion: fail to delete (%s)'s pod and svc: %v", pods.Items[i].Name, err)
			}
		}
	}

	c.deleteClientServiceLB()
}

func (c *Cluster) deleteClientServiceLB() {
	if err := c.deleteClientService(fmt.Sprintf("%s-pd", c.Name)); err != nil {
		log.Errorf("cluster deletion: fail to delete client service LB: %v", err)
	}
}

func (c *Cluster) deleteClientService(name string) error {
	err := c.KubeCli.Services(c.Namespace).Delete(name)
	if err != nil {
		if !util.IsKubernetesResourceNotFoundError(err) {
			return errors.Trace(err)
		}
	}

	return nil
}

func (c *Cluster) pollPods(tp member.MemberType) ([]*k8sapi.Pod, []*k8sapi.Pod, error) {
	podList, err := c.KubeCli.Pods(c.Namespace).List(member.PodListOpt(c.Name, tp))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list running ServiceType: %v pods: %v", tp, err)
	}

	var running []*k8sapi.Pod
	var pending []*k8sapi.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		switch pod.Status.Phase {
		case k8sapi.PodRunning:
			running = append(running, pod)
		case k8sapi.PodPending:
			pending = append(pending, pod)
		}
	}

	return running, pending, nil
}
