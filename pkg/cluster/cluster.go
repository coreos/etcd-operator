package cluster

import (
	"fmt"

	"github.com/nolouch/tidb-oprator/cluster/member"
	"github.com/nolouch/tidb-oprator/spec"

	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

type clusterEventType string
type Config struct {
	Name      string
	Namespace string
	KubeCli   *unversioned.Client
}

type Status struct {
	CurrentVersion string `json:"currentVersion"`
	TargetVersion  string `json:"targetVersion"`
}

type clusterEvent struct {
	typ  clusterEventType
	spec spec.ClusterSpec
}

type Cluster struct {
	members map[MemberType]MemberSet

	Config

	status *Status

	spec      *spec.ClusterSpec
	idCounter map[MemberType]int
	eventCh   chan *clusterEvent
	stopCh    chan struct{}
}

func New(c Config, s *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup) (*Cluster, error) {
	return new(c, s, stopC, wg, true)
}

func new(config Config, s *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup) (*Cluster, error) {

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

	c.prepareSeedMember()

	wg.Add(1)
	go c.run(stopC, wg)

	return c, nil
}

func (c *Cluster) prepareSeedMember() error {
	ms := member.SeedPDMemSet(s)
	c.addMemberSet(ms)

}

func (c *Cluster) addMemberSet(ms member.MemberSet) {
	typ := ms.Type()
	c.members[typ] = ms
}

func (c *Cluster) run(stopC <-chan struct{}, wg *sync.WaitGroup) {
	needDeleteCluster := true

	for t, ms := range c.members {
		wg.Add(1)
		go ms.Run()
	}

	defer func() {
		if needDeleteCluster {
			c.logger.Infof("deleting cluster")
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
			case eventModifyPD:
				ms, _ := c.members[PD]
				ms.SetSepc(event.spec)
			case eventModifyTiDB:
			case eventModifyTiKV:
			case eventDeleteCluster:
				return
			}
		}
	}
}
