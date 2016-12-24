package cluster

import (
	"fmt"

	"github.com/GregoryIan/operator/util/k8sutil"
	"github.com/GregoryIan/oprator/cluster/member"
	"github.com/GregoryIan/oprator/spec"

	"github.com/juju/errors"
	"github.com/ngaut/log"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

const defaultVersion = "rc1"

type clusterEventType string

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

func New(config Config, s *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup) (*Cluster, error) {
	if len(s.Version) == 0 {
		s.Version = defaultVersion
	}

	c := &Cluster{
		Config:  config,
		spec:    s,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
		status:  &Status{},
	}

	if err := c.prepareSeedMember(); err != nil {
		return nil, err
	}
	if err := c.createClientServiceLB(); err != nil {
		return nil, fmt.Errorf("fail to create client service LB: %v", err)
	}

	go c.run(stopC, wg)

	return c, nil
}

func (c *Cluster) prepareSeedMember() error {
	members, err := member.InitSeedMembers(c.Name, c.Namespace, c.KubeCli)
	if err != nil {
		return err
	}

	c.members = members
	return nil
}

func (c *Cluster) run(stopC <-chan struct{}, wg *sync.WaitGroup) {
	wg.Add(1)
	defer func() {
		if needDeleteCluster {
			c.logger.Infof("deleting cluster")
			c.delete()
		}
		close(c.stopCh)
		wg.Done()
	}()

	needDeleteCluster := true
	for {
		select {
		case <-stopC:
			needDeleteCluster = false
			return
		case event := <-c.eventCh:
			switch event.typ {
			case eventModifyCluster:
				c.logger.Infof("spec update: from: %v to: %v", c.spec, event.spec)
				c.spec = &event.spec
				c.splitAndDistributeSpec()
			case eventDeleteCluster:
				return
			}
		}
	}
}

func (c *Cluster) createClientServiceLB() error {
	if _, err := k8sutil.CreateService(c.KubeCli, fmt.Sprintf("%s-pd", c.Name), c.Namespace, k8sutil.PD); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	if _, err := k8sutil.CreateService(c.KubeCli, fmt.Sprintf("%s-tidb", c.Name), c.Namespace, k8sutil.TiDB); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}

	return nil
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
			if err := c.removePodAndService(pods.Items[i].Name); err != nil {
				log.Errorf("cluster deletion: fail to delete (%s)'s pod and svc: %v", pods.Items[i].Name, err)
			}
		}
	}

	err = c.deleteClientServiceLB()
	if err != nil {
		log.Errorf("cluster deletion: fail to delete client service LB: %v", err)
	}
}

func (c *Cluster) removePodAndService(name string) error {
	err := c.KubeCli.Services(c.Namespace).Delete(name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	err = c.KubeCli.Pods(c.Namespace).Delete(name, k8sapi.NewDeleteOptions(0))
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) removePodAndService(name string) error {
	err := c.KubeCli.Services(c.Namespace).Delete(name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	err = c.KubeCli.Pods(c.Namespace).Delete(name, k8sapi.NewDeleteOptions(0))
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) deleteClientServiceLB() error {
	if err := c.deleteClientService(fmt.Sprintf("%s-pd", c.Name)); err != nil {
		return err
	}

	return c.deleteClientService(fmt.Sprintf("%s-tidb", c.Name))
}

func (c *Cluster) deleteClientService(name string) error {
	err := c.KubeCli.Services(c.Namespace).Delete(name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}

	return nil
}
