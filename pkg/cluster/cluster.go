// Copyright 2016 The kube-etcd-controller Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cluster

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/kube-etcd-controller/pkg/spec"
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

	defaultVersion = "v3.1.0-alpha.1"
)

type clusterEvent struct {
	typ  clusterEventType
	spec spec.ClusterSpec
}

type Cluster struct {
	kclient *unversioned.Client

	status *Status

	spec *spec.ClusterSpec

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

func New(c *unversioned.Client, name, ns string, spec *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup) *Cluster {
	return new(c, name, ns, spec, stopC, wg, true)
}

func Restore(c *unversioned.Client, name, ns string, spec *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup) *Cluster {
	return new(c, name, ns, spec, stopC, wg, false)
}

func new(kclient *unversioned.Client, name, ns string, spec *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup, isNewCluster bool) *Cluster {
	if len(spec.Version) == 0 {
		// TODO: set version in spec in apiserver
		spec.Version = defaultVersion
	}
	c := &Cluster{
		kclient:   kclient,
		name:      name,
		namespace: ns,
		eventCh:   make(chan *clusterEvent, 100),
		stopCh:    make(chan struct{}),
		spec:      spec,
		status:    &Status{},
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
	wg.Add(1)
	go c.run(stopC, wg)

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

func (c *Cluster) run(stopC <-chan struct{}, wg *sync.WaitGroup) {
	needDeleteCluster := true

	defer func() {
		if needDeleteCluster {
			log.Warningf("deleting cluster (%v)", c.name)
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
				// TODO: we can't handle another upgrade while an upgrade is in progress
				log.Printf("spec update: from: %v, to: %v", c.spec, event.spec)
				c.spec = &event.spec
			case eventDeleteCluster:
				return
			}
		case <-time.After(5 * time.Second):
			running, pending, err := c.pollPods()
			if err != nil {
				log.Errorf("cluster (%v) fail to poll pods: %v", c.name, err)
				continue
			}
			if len(pending) > 0 {
				log.Infof("cluster (%v) skip reconciliation: running (%v), pending (%v)", c.name, k8sutil.GetPodNames(running), k8sutil.GetPodNames(pending))
				continue
			}
			if len(running) == 0 {
				err := c.disasterRecovery(nil)
				if err != nil {
					if err == errNoBackupExist {
						panic("TODO: mark cluster dead if no backup for disaster recovery.")
					}
					log.Errorf("cluster (%v) fail to recover. Will retry later: %v", c.name, err)
				}
				continue // Backoff, either on normal recovery or error.
			}

			if err := c.reconcile(running); err != nil {
				log.Errorf("cluster (%v) fail to reconcile: %v", c.name, err)
				if isFatalError(err) {
					log.Errorf("cluster (%v) had fatal error: %v", c.name, err)
					return
				}
			}
		}
	}
}

func isFatalError(err error) bool {
	switch err {
	case errNoBackupExist:
		return true
	default:
		return false
	}
}

func (c *Cluster) makeSeedMember() *etcdutil.Member {
	etcdName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	return &etcdutil.Member{Name: etcdName}
}

func (c *Cluster) startSeedMember(recoverFromBackup bool) error {
	m := c.makeSeedMember()
	if err := c.createPodAndService(etcdutil.NewMemberSet(m), m, "new", recoverFromBackup); err != nil {
		log.Errorf("failed to create seed member (%s): %v", m.Name, err)
		return err
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
	log.Infof("migrating seed member (%s)", c.spec.Seed.MemberClientEndpoints)

	cfg := clientv3.Config{
		Endpoints:   c.spec.Seed.MemberClientEndpoints,
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberList(ctx)
	cancel()
	if err != nil {
		return err
	}
	if len(resp.Members) != 1 {
		return fmt.Errorf("seed cluster contains more than one member")
	}
	seedMember := resp.Members[0]
	seedID := resp.Members[0].ID

	log.Infof("adding a new member")

	// create the first member inside Kubernetes for migration
	etcdName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	m := &etcdutil.Member{Name: etcdName}

	svc, err := k8sutil.CreateEtcdService(c.kclient, m.Name, c.name, c.namespace)
	if err != nil {
		return err
	}
	log.Info("created etcd service")

	// Used the service IP as additional peer URL so that the seed member can access it.
	m.AdditionalPeerURL = fmt.Sprintf("http://%s:2380", svc.Spec.ClusterIP)

	mpurls := []string{fmt.Sprintf("http://%s:2380", m.Name), m.AdditionalPeerURL}

	initialCluster := make([]string, 0)
	for _, purl := range seedMember.PeerURLs {
		initialCluster = append(initialCluster, fmt.Sprintf("%s=%s", seedMember.Name, purl))
	}
	for _, purl := range mpurls {
		initialCluster = append(initialCluster, fmt.Sprintf("%s=%s", m.Name, purl))
	}

	pod := k8sutil.MakeEtcdPod(m, initialCluster, c.name, "existing", "", c.spec)
	pod = k8sutil.PodWithAddMemberInitContainer(pod, m.Name, mpurls, c.spec)

	if _, err := c.kclient.Pods(c.namespace).Create(pod); err != nil {
		return err
	}
	c.idCounter++
	etcdcli.Close()
	log.Infof("added the new member")

	// wait for the delay
	if c.spec.Seed.RemoveDelay == 0 {
		c.spec.Seed.RemoveDelay = 30
	}
	if c.spec.Seed.RemoveDelay < 10 {
		c.spec.Seed.RemoveDelay = 10
	}

	delay := time.Duration(c.spec.Seed.RemoveDelay) * time.Second
	log.Infof("wait %v before remove the seed member", delay)
	time.Sleep(delay)

	log.Infof("removing the seed member")

	cfg = clientv3.Config{
		Endpoints:   []string{m.ClientAddr()},
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err = clientv3.New(cfg)
	if err != nil {
		return err
	}

	// delete the original seed member from the etcd cluster.
	// now we have migrate the seed member into kubernetes.
	// our controller not takes control over it.
	ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	_, err = etcdcli.Cluster.MemberRemove(ctx, seedID)
	cancel()
	if err != nil {
		return fmt.Errorf("etcdcli failed to remove seed member: %v", err)
	}

	log.Infof("removed the seed member")

	ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err = etcdcli.MemberList(ctx)
	cancel()
	if err != nil {
		return err
	}

	log.Infof("updating the peer urls (%v) for the member %x", resp.Members[0].PeerURLs, resp.Members[0].ID)

	m.AdditionalPeerURL = ""
	for {
		ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
		_, err = etcdcli.MemberUpdate(ctx, resp.Members[0].ID, []string{m.PeerAddr()})
		cancel()
		if err == nil {
			break
		}
		if err != context.DeadlineExceeded {
			return err
		}
	}

	etcdcli.Close()
	log.Infof("finished the member migration")

	return nil
}

func (c *Cluster) Update(spec *spec.ClusterSpec) {
	anyInterestedChange := false
	if spec.Size != c.spec.Size {
		anyInterestedChange = true
	}
	if len(spec.Version) == 0 {
		spec.Version = defaultVersion
	}
	if spec.Version != c.spec.Version {
		anyInterestedChange = true
	}
	if anyInterestedChange {
		c.send(&clusterEvent{
			typ:  eventModifyCluster,
			spec: *spec,
		})
	}
}

func (c *Cluster) updateMembers(etcdcli *clientv3.Client) error {
	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberList(ctx)
	if err != nil {
		return err
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
	return nil
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
	if c.spec.Backup != nil {
		k8sutil.DeleteBackupReplicaSetAndService(c.kclient, c.name, c.namespace, c.spec.Backup.CleanupBackupIfDeleted)
	}
}

func (c *Cluster) createPodAndService(members etcdutil.MemberSet, m *etcdutil.Member, state string, needRecovery bool) error {
	// TODO: remove garbage service. Because we will fail after service created before pods created.
	if _, err := k8sutil.CreateEtcdService(c.kclient, m.Name, c.name, c.namespace); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	token := ""
	if state == "new" {
		token = uuid.New()
	}
	pod := k8sutil.MakeEtcdPod(m, members.PeerURLPairs(), c.name, state, token, c.spec)
	if needRecovery {
		k8sutil.AddRecoveryToPod(pod, c.name, m.Name, token, c.spec)
	}
	_, err := c.kclient.Pods(c.namespace).Create(pod)
	return err
}

func (c *Cluster) removePodAndService(name string) error {
	err := c.kclient.Services(c.namespace).Delete(name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	err = c.kclient.Pods(c.namespace).Delete(name, k8sapi.NewDeleteOptions(0))
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) pollPods() ([]*k8sapi.Pod, []*k8sapi.Pod, error) {
	podList, err := c.kclient.Pods(c.namespace).List(k8sutil.EtcdPodListOpt(c.name))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list running pods: %v", err)
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
