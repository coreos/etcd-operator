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
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	apierrors "k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/v1"
)

var reconcileInterval = 8 * time.Second

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
	KubeCli    kubernetes.Interface
}

type Cluster struct {
	logger *logrus.Entry

	config Config

	cluster *spec.EtcdCluster

	// in memory state of the cluster
	// status is the source of truth after Cluster struct is materialized.
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
		status:  e.Status,
		gc:      garbagecollection.New(config.KubeCli, e.Namespace),
	}

	if c.status == nil {
		c.status = &spec.ClusterStatus{}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := c.setup()
		if err != nil {
			c.logger.Errorf("cluster failed to setup: %v", err)
			if c.status.Phase != spec.ClusterPhaseFailed {
				c.status.SetReason(err.Error())
				c.status.SetPhase(spec.ClusterPhaseFailed)
				if err := c.updateStatus(); err != nil {
					c.logger.Errorf("failed to update cluster phase (%v): %v", spec.ClusterPhaseFailed, err)
				}
			}
			return
		}
		c.run(stopC)
	}()

	return c
}

func (c *Cluster) setup() error {
	err := c.cluster.Spec.Validate()
	if err != nil {
		return fmt.Errorf("invalid cluster spec: %v", err)
	}

	var shouldCreateCluster bool
	switch c.status.Phase {
	case spec.ClusterPhaseNone:
		shouldCreateCluster = true
	case spec.ClusterPhaseCreating:
		return errors.New("cluster failed to be created")
	case spec.ClusterPhaseRunning:
		shouldCreateCluster = false

	default:
		return fmt.Errorf("unexpected cluster phase: %s", c.status.Phase)
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
	c.logger.Infof("creating cluster with Spec (%#v), Status (%#v)", c.cluster.Spec, c.cluster.Status)
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

func (c *Cluster) run(stopC <-chan struct{}) {
	clusterFailed := false

	defer func() {
		if clusterFailed {
			c.reportFailedStatus()

			c.logger.Infof("deleting the failed cluster")
			c.delete()
		}

		close(c.stopCh)
	}()

	c.status.SetPhase(spec.ClusterPhaseRunning)
	if err := c.updateStatus(); err != nil {
		c.logger.Warningf("failed to update TPR status: %v", err)
	}

	var rerr error
	for {
		select {
		case <-stopC:
			return
		case event := <-c.eventCh:
			switch event.typ {
			case eventModifyCluster:
				if isSpecEqual(event.cluster.Spec, c.cluster.Spec) {
					break
				}
				// TODO: we can't handle another upgrade while an upgrade is in progress
				c.logger.Infof("spec update: from: %v to: %v", c.cluster.Spec, event.cluster.Spec)
				c.cluster = event.cluster

			case eventDeleteCluster:
				c.logger.Infof("cluster is deleted by the user")
				clusterFailed = true
				return
			}

		case <-time.After(reconcileInterval):
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

			// On controller restore, we could have "members == nil"
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

			// TODO: We are coupling GC logic here because we need to sync with current membership.
			// - We need to redesign this GC part and have it not coupled with cluster code
			// - This is listing serivces every reconcile loop. We need to reduce such performance impact.
			garbagecollection.RemoveDeletedMemberServices(c.cluster.Name, c.cluster.Namespace, c.config.KubeCli, c.members, c.logger)

			if err := c.updateStatus(); err != nil {
				c.logger.Warningf("failed to update TPR status: %v", err)
			}
		}

		if isFatalError(rerr) {
			clusterFailed = true
			c.status.SetReason(rerr.Error())

			c.logger.Errorf("cluster failed: %v", rerr)
			return
		}
	}
}

func isSpecEqual(s1, s2 *spec.ClusterSpec) bool {
	if s1.Size != s2.Size || s1.Paused != s2.Paused || s1.Version != s2.Version {
		return false
	}
	return true
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
	ms := etcdutil.NewMemberSet(m)
	if err := c.createPodAndService(ms, m, "new", recoverFromBackup); err != nil {
		return fmt.Errorf("failed to create seed member (%s): %v", m.Name, err)
	}
	c.memberCounter++
	c.members = ms
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
	c.send(&clusterEvent{
		typ:     eventModifyCluster,
		cluster: e,
	})
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
	err := c.config.KubeCli.Core().Services(c.cluster.Namespace).Delete(c.cluster.Name, nil)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) createPodAndService(members etcdutil.MemberSet, m *etcdutil.Member, state string, needRecovery bool) error {
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
	_, err := c.config.KubeCli.Core().Pods(c.cluster.Namespace).Create(pod)
	return err
}

func (c *Cluster) removePodAndService(name string) error {
	err := c.config.KubeCli.Core().Services(c.cluster.Namespace).Delete(name, nil)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	err = c.config.KubeCli.Core().Pods(c.cluster.Namespace).Delete(name, api.NewDeleteOptions(0))
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) pollPods() ([]*v1.Pod, []*v1.Pod, error) {
	podList, err := c.config.KubeCli.Core().Pods(c.cluster.Namespace).List(k8sutil.ClusterListOpt(c.cluster.Name))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list running pods: %v", err)
	}

	var running []*v1.Pod
	var pending []*v1.Pod
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
		case v1.PodRunning:
			running = append(running, pod)
		case v1.PodPending:
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
	newCluster, err := k8sutil.UpdateClusterTPRObject(c.config.KubeCli.Core().GetRESTClient(), c.cluster.GetNamespace(), newCluster)
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
		if err == nil || k8sutil.IsKubernetesResourceNotFoundError(err) {
			return true, nil
		}

		if apierrors.IsConflict(err) {
			cl, err := k8sutil.GetClusterTPRObject(c.config.KubeCli.Core().GetRESTClient(), c.cluster.Namespace, c.cluster.Name)
			if err != nil {
				c.logger.Warningf("report status: fail to get latest version: %v", err)
				return false, nil
			}
			c.cluster = cl
			return false, nil
		}
		c.logger.Warningf("report status: fail to update: %v", err)
		return false, nil
	}

	retryutil.Retry(5*time.Second, math.MaxInt64, f)
}
