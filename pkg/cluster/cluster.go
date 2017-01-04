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
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/garbagecollection"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	"github.com/Sirupsen/logrus"
	"github.com/pborman/uuid"
	k8sapi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type clusterEventType string

const (
	eventDeleteCluster clusterEventType = "Delete"
	eventModifyCluster clusterEventType = "Modify"
)

type clusterEvent struct {
	typ     clusterEventType
	cluster *spec.EtcdCluster
}

type Config struct {
	PVProvisioner string
	s3config.S3Context

	MasterHost string
	KubeCli    *unversioned.Client
}

type Cluster struct {
	logger *logrus.Entry

	config Config

	cluster *spec.EtcdCluster

	// in memory state of the cluster
	status        *spec.ClusterStatus
	memberCounter int

	eventCh chan *clusterEvent
	stopCh  chan struct{}

	// members repsersents the members in the etcd cluster.
	// the name of the member is the the name of the pod the member
	// process runs in.
	members etcdutil.MemberSet

	bm        *backupManager
	backupDir string

	gc *garbagecollection.GC
}

func New(config Config, e *spec.EtcdCluster, stopC <-chan struct{}, wg *sync.WaitGroup) *Cluster {
	lg := logrus.WithField("pkg", "cluster").WithField("cluster-name", e.Name)
	c := &Cluster{
		logger:  lg,
		config:  config,
		cluster: e,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
		status:  &spec.ClusterStatus{},
		gc:      garbagecollection.New(config.KubeCli, config.MasterHost, e.Namespace),
	}

	err := c.setup()
	if err != nil {
		c.logger.Errorf("fail to setup: %v", err)
		if c.status.Phase != spec.ClusterPhaseFailed {
			c.status.SetReason(err.Error())
			c.status.SetPhase(spec.ClusterPhaseFailed)
			if err := c.updateStatus(); err != nil {
				c.logger.Errorf("failed to update cluster phase (%v): %v", spec.ClusterPhaseFailed, err)
			}
		}
		return nil
	}

	wg.Add(1)
	go c.run(stopC, wg)
	return c
}

func (c *Cluster) setup() error {
	err := c.cluster.Spec.Validate()
	if err != nil {
		return fmt.Errorf("invalid cluster spec: %v", err)
	}

	var phase spec.ClusterPhase
	if c.cluster.Status != nil {
		phase = c.cluster.Status.Phase
	}
	var shouldCreateCluster bool
	switch phase {
	case "":
		shouldCreateCluster = true
	case spec.ClusterPhaseCreating:
		return errors.New("cluster failed to be created")
	case spec.ClusterPhaseRunning:
		shouldCreateCluster = false
	case spec.ClusterPhaseFailed:
		return errors.New("cluster already failed")
	}

	if b := c.cluster.Spec.Backup; b != nil && b.MaxBackups > 0 {
		c.bm, err = newBackupManager(c.config, c.cluster, c.logger)
		if err != nil {
			return err
		}
	}

	if shouldCreateCluster {
		return c.create()
	}
	return nil
}

func (c *Cluster) create() error {
	c.status.SetPhase(spec.ClusterPhaseCreating)
	if err := c.updateStatus(); err != nil {
		return fmt.Errorf("cluster create: failed to update cluster phase (%v): %v", spec.ClusterPhaseCreating, err)
	}

	if err := c.gc.CollectCluster(c.cluster.Name, c.cluster.UID); err != nil {
		return fmt.Errorf("cluster create: failed to clean up conflict resources: %v", err)
	}

	if c.bm != nil {
		if err := c.bm.setup(); err != nil {
			return err
		}
	}

	if c.cluster.Spec.Restore == nil {
		// Note: For restore case, we don't need to create seed member,
		// and will go through reconcile loop and disaster recovery.
		if err := c.prepareSeedMember(); err != nil {
			return err
		}
	}

	if err := c.createClientServiceLB(); err != nil {
		return fmt.Errorf("cluster create: fail to create client service LB: %v", err)
	}
	return nil
}

func (c *Cluster) prepareSeedMember() error {
	var err error
	if sh := c.cluster.Spec.SelfHosted; sh != nil {
		if len(sh.BootMemberClientEndpoint) == 0 {
			err = c.newSelfHostedSeedMember()
		} else {
			err = c.migrateBootMember()
		}
	} else {
		err = c.newSeedMember()
	}
	return err
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

		c.reportFailedStatus()
	}()

	c.status.SetPhase(spec.ClusterPhaseRunning)

	var rerr error
	for {
		select {
		case <-stopC:
			needDeleteCluster = false
			return
		case event := <-c.eventCh:
			switch event.typ {
			case eventModifyCluster:
				// TODO: we can't handle another upgrade while an upgrade is in progress
				c.logger.Infof("spec update: from: %v to: %v", c.cluster.Spec, event.cluster.Spec)
				c.cluster = event.cluster
			case eventDeleteCluster:
				return
			}
		case <-time.After(5 * time.Second):
			if c.cluster.Spec.Paused {
				c.status.PauseControl()
				c.logger.Infof("control is paused, skipping reconcilation")
				continue
			} else {
				c.status.Control()
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
				rerr = c.disasterRecovery(nil)
				if rerr != nil {
					c.logger.Errorf("fail to do disaster recovery: %v", rerr)
				}
				// On normal recovery case, we need backoff. On error case, this could be either backoff or leading to cluster delete.
				break
			}

			// TODO: The case "c.members = nil" only happens on creating seed member.
			//       We should just set the member directly if successful.
			if rerr != nil || c.members == nil {
				rerr = c.updateMembers(podsToMemberSet(running, c.cluster.Spec.SelfHosted))
				if rerr != nil {
					c.logger.Errorf("failed to update members: %v", rerr)
					break
				}
			}
			rerr = c.reconcile(running)
			if rerr != nil {
				c.logger.Errorf("failed to reconcile: %v", rerr)
				break
			}

			if err := c.updateStatus(); err != nil {
				c.logger.Warningf("failed to update TPR status: %v", err)
			}
		}

		if isFatalError(rerr) {
			c.cluster.Status.SetReason(rerr.Error())
			c.logger.Errorf("exiting for fatal error: %v", rerr)
			return
		}
	}
}

func isFatalError(err error) bool {
	switch err {
	case errNoBackupExist, errInvalidMemberName, errUnexpectedUnreadyMember:
		return true
	default:
		return false
	}
}

func (c *Cluster) makeSeedMember() *etcdutil.Member {
	etcdName := etcdutil.CreateMemberName(c.cluster.Name, c.memberCounter)
	return &etcdutil.Member{Name: etcdName}
}

func (c *Cluster) startSeedMember(recoverFromBackup bool) error {
	m := c.makeSeedMember()
	if err := c.createPodAndService(etcdutil.NewMemberSet(m), m, "new", recoverFromBackup); err != nil {
		c.logger.Errorf("failed to create seed member (%s): %v", m.Name, err)
		return err
	}
	c.memberCounter++
	c.logger.Infof("cluster created with seed member (%s)", m.Name)
	return nil
}

func (c *Cluster) newSeedMember() error {
	return c.startSeedMember(false)
}

func (c *Cluster) restoreSeedMember() error {
	return c.startSeedMember(true)
}

func (c *Cluster) Update(e *spec.EtcdCluster) {
	anyInterestedChange := false
	s1, s2 := e.Spec, c.cluster.Spec
	switch {
	case s1.Size != s2.Size, s1.Paused != s2.Paused, s1.Version != s2.Version:
		anyInterestedChange = true
	}
	if anyInterestedChange {
		c.send(&clusterEvent{
			typ:     eventModifyCluster,
			cluster: e,
		})
	}
}

func (c *Cluster) delete() {
	if err := c.gc.CollectCluster(c.cluster.Name, garbagecollection.NullUID); err != nil {
		c.logger.Errorf("cluster delete: fail to clean up resources %v", err)
	}

	if c.bm != nil {
		err := c.bm.cleanup()
		if err != nil {
			c.logger.Errorf("cluster deletion: backup manager failed to cleanup: %v", err)
		}
	}
}

func (c *Cluster) createClientServiceLB() error {
	if _, err := k8sutil.CreateEtcdService(c.config.KubeCli, c.cluster.Name, c.cluster.Namespace, c.cluster.AsOwner()); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) deleteClientServiceLB() error {
	err := c.config.KubeCli.Services(c.cluster.Namespace).Delete(c.cluster.Name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) createPodAndService(members etcdutil.MemberSet, m *etcdutil.Member, state string, needRecovery bool) error {
	// TODO: remove garbage service. Because we will fail after service created before pods created.
	svc := k8sutil.MakeEtcdMemberService(m.Name, c.cluster.Name, c.cluster.AsOwner())
	if _, err := k8sutil.CreateEtcdMemberService(c.config.KubeCli, c.cluster.Namespace, svc); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	token := ""
	if state == "new" {
		token = uuid.New()
	}

	pod := k8sutil.MakeEtcdPod(m, members.PeerURLPairs(), c.cluster.Name, state, token, c.cluster.Spec, c.cluster.AsOwner())
	if needRecovery {
		k8sutil.AddRecoveryToPod(pod, c.cluster.Name, m.Name, token, c.cluster.Spec)
	}
	_, err := c.config.KubeCli.Pods(c.cluster.Namespace).Create(pod)
	return err
}

func (c *Cluster) removePodAndService(name string) error {
	err := c.config.KubeCli.Services(c.cluster.Namespace).Delete(name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	err = c.config.KubeCli.Pods(c.cluster.Namespace).Delete(name, k8sapi.NewDeleteOptions(0))
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) pollPods() ([]*k8sapi.Pod, []*k8sapi.Pod, error) {
	podList, err := c.config.KubeCli.Pods(c.cluster.Namespace).List(k8sutil.ClusterListOpt(c.cluster.Name))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list running pods: %v", err)
	}

	var running []*k8sapi.Pod
	var pending []*k8sapi.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if len(pod.OwnerReferences) < 1 {
			c.logger.Warningf("pollPods: ignore pod %v: no owner", pod.Name)
			continue
		}
		if pod.OwnerReferences[0].UID != c.cluster.UID {
			c.logger.Warningf("pollPods: ignore pod %v: owner (%v) is not %v", pod.Name, pod.OwnerReferences[0].UID, c.cluster.UID)
			continue
		}
		switch pod.Status.Phase {
		case k8sapi.PodRunning:
			running = append(running, pod)
		case k8sapi.PodPending:
			pending = append(pending, pod)
		}
	}

	return running, pending, nil
}

func (c *Cluster) updateStatus() error {
	if reflect.DeepEqual(c.cluster.Status, c.status) {
		return nil
	}

	newCluster := c.cluster
	newCluster.Status = c.status
	newCluster, err := k8sutil.UpdateClusterTPRObject(c.config.KubeCli, c.config.MasterHost, c.cluster.GetNamespace(), newCluster)
	if err != nil {
		return err
	}

	c.cluster = newCluster
	return nil
}

func (c *Cluster) reportFailedStatus() {
	f := func() (bool, error) {
		c.status.SetPhase(spec.ClusterPhaseFailed)
		err := c.updateStatus()
		switch err {
		case nil, k8sutil.ErrTPRObjectNotFound:
			return true, nil
		case k8sutil.ErrTPRObjectVersionConflict:
			cl, err := k8sutil.GetClusterTPRObject(c.config.KubeCli, c.config.MasterHost, c.cluster.Namespace, c.cluster.Name)
			if err != nil {
				c.logger.Warningf("report status: fail to get latest version: %v", err)
				return false, nil
			}
			c.cluster = cl
		}
		c.logger.Warningf("report status: fail to update: %v", err)
		return false, nil
	}

	retryutil.Retry(5*time.Second, math.MaxInt64, f)
}
