// Copyright 2016 The etcd-operator Authors
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

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
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
	logger *logrus.Entry

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
		logger:    logrus.WithField("pkg", "cluster").WithField("cluster-name", name),
		kclient:   kclient,
		name:      name,
		namespace: ns,
		eventCh:   make(chan *clusterEvent, 100),
		stopCh:    make(chan struct{}),
		spec:      spec,
		status:    &Status{},
	}
	if isNewCluster {
		err := c.createClientServiceLB()
		if err != nil {
			// todo: do not panic!
			panic("todo:" + err.Error())
		}
		if c.spec.SelfHosted != nil {
			if len(c.spec.SelfHosted.BootMemberClientEndpoint) == 0 {
				err = c.newSelfHostedSeedMember()
			} else {
				err = c.migrateBootMember()
			}
		} else {
			err = c.newSeedMember()
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
			case eventModifyCluster:
				// TODO: we can't handle another upgrade while an upgrade is in progress
				c.logger.Infof("spec update: from: %v to: %v", c.spec, event.spec)
				c.spec = &event.spec
			case eventDeleteCluster:
				return
			}
		case <-time.After(5 * time.Second):
			if c.spec.Paused {
				c.logger.Infof("control is paused, skipping reconcilation")
				continue
			}

			running, pending, err := c.pollPods()
			if err != nil {
				c.logger.Errorf("fail to poll pods: %v", err)
				continue
			}
			if len(pending) > 0 {
				c.logger.Infof("skip reconciliation: running (%v), pending (%v)", k8sutil.GetPodNames(running), k8sutil.GetPodNames(pending))
				continue
			}
			if len(running) == 0 {
				c.logger.Warningf("all etcd pods are dead. Trying to recover from a previous backup")
				err := c.disasterRecovery(nil)
				if err != nil {
					if err == errNoBackupExist {
						panic("TODO: mark cluster dead if no backup for disaster recovery.")
					}
					c.logger.Errorf("fail to recover. Will retry later: %v", err)
				}
				continue // Back-off, either on normal recovery or error.
			}

			if err := c.reconcile(running); err != nil {
				c.logger.Errorf("fail to reconcile: %v", err)
				if isFatalError(err) {
					c.logger.Errorf("exiting for fatal error: %v", err)
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
		c.logger.Errorf("failed to create seed member (%s): %v", m.Name, err)
		return err
	}
	c.logger.Infof("cluster created with seed member (%s)", m.Name)
	return nil
}

func (c *Cluster) newSeedMember() error {
	return c.startSeedMember(false)
}

func (c *Cluster) restoreSeedMember() error {
	return c.startSeedMember(true)
}

func (c *Cluster) Update(spec *spec.ClusterSpec) {
	anyInterestedChange := false
	if (spec.Size != c.spec.Size) || (spec.Paused != c.spec.Paused) {
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
		if len(m.Name) == 0 {
			c.members = nil
			return fmt.Errorf("the name of member (%x) is empty. Not ready yet. Will retry later", m.ID)
		}
		id := findID(m.Name)
		if id+1 > c.idCounter {
			c.idCounter = id + 1
		}

		c.members[m.Name] = &etcdutil.Member{
			Name:       m.Name,
			ID:         m.ID,
			ClientURLs: m.ClientURLs,
			PeerURLs:   m.PeerURLs,
		}
	}
	return nil
}

func findID(name string) int {
	i := strings.LastIndex(name, "-")
	id, err := strconv.Atoi(name[i+1:])
	if err != nil {
		// TODO: do not panic for single cluster error
		panic(fmt.Sprintf("TODO: fail to extract valid ID from name (%s): %v", name, err))
	}
	return id
}

func (c *Cluster) delete() {
	option := k8sapi.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": c.name,
			"app":          "etcd",
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
		k8sutil.DeleteBackupReplicaSetAndService(c.kclient, c.name, c.namespace, c.spec.Backup.CleanupOnClusterDelete)
	}

	err = c.deleteClientServiceLB()
	if err != nil {
		// todo: do not panic!
		panic("todo:" + err.Error())
	}
}

func (c *Cluster) createClientServiceLB() error {
	if _, err := k8sutil.CreateEtcdService(c.kclient, c.name, c.namespace); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) deleteClientServiceLB() error {
	err := c.kclient.Services(c.namespace).Delete(c.name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) createPodAndService(members etcdutil.MemberSet, m *etcdutil.Member, state string, needRecovery bool) error {
	// TODO: remove garbage service. Because we will fail after service created before pods created.
	if _, err := k8sutil.CreateEtcdMemberService(c.kclient, m.Name, c.name, c.namespace); err != nil {
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
	if err != nil {
		return err
	}
	c.idCounter++
	return nil
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
