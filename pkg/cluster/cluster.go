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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

var (
	reconcileInterval         = 8 * time.Second
	podTerminationGracePeriod = int64(5)
)

type clusterEventType string

const (
	eventDeleteCluster clusterEventType = "Delete"
	eventModifyCluster clusterEventType = "Modify"
)

type clusterEvent struct {
	typ     clusterEventType
	cluster *spec.Cluster
}

type Config struct {
	PVProvisioner  string
	ServiceAccount string
	s3config.S3Context

	KubeCli kubernetes.Interface
}

type Cluster struct {
	logger *logrus.Entry

	config Config

	cluster *spec.Cluster

	// in memory state of the cluster
	// status is the source of truth after Cluster struct is materialized.
	status        spec.ClusterStatus
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

func New(config Config, cl *spec.Cluster, stopC <-chan struct{}, wg *sync.WaitGroup) *Cluster {
	lg := logrus.WithField("pkg", "cluster").WithField("cluster-name", cl.Metadata.Name)
	c := &Cluster{
		logger:  lg,
		config:  config,
		cluster: cl,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
		status:  cl.Status.Copy(),
		gc:      garbagecollection.New(config.KubeCli, cl.Metadata.Namespace),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := c.setup(); err != nil {
			c.logger.Errorf("cluster failed to setup: %v", err)
			if c.status.Phase != spec.ClusterPhaseFailed {
				c.status.SetReason(err.Error())
				c.status.SetPhase(spec.ClusterPhaseFailed)
				if err := c.updateTPRStatus(); err != nil {
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
		return errCreatedCluster
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
	c.status.SetPhase(spec.ClusterPhaseCreating)

	if err := c.updateTPRStatus(); err != nil {
		return fmt.Errorf("cluster create: failed to update cluster phase (%v): %v", spec.ClusterPhaseCreating, err)
	}
	c.logger.Infof("creating cluster with Spec (%#v), Status (%#v)", c.cluster.Spec, c.cluster.Status)

	c.gc.CollectCluster(c.cluster.Metadata.Name, c.cluster.Metadata.UID)

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

	if err := c.setupServices(); err != nil {
		return fmt.Errorf("cluster create: fail to create client service LB: %v", err)
	}
	return nil
}

func (c *Cluster) prepareSeedMember() error {
	c.status.AppendScalingUpCondition(0, c.cluster.Spec.Size)

	var err error
	if sh := c.cluster.Spec.SelfHosted; sh != nil {
		if len(sh.BootMemberClientEndpoint) == 0 {
			err = c.newSelfHostedSeedMember()
		} else {
			err = c.migrateBootMember()
		}
	} else {
		err = c.bootstrap()
	}
	if err != nil {
		return err
	}

	c.status.Size = 1
	return nil
}

func (c *Cluster) Delete() {
	c.send(&clusterEvent{typ: eventDeleteCluster})
}

func (c *Cluster) send(ev *clusterEvent) {
	select {
	case c.eventCh <- ev:
		l, ecap := len(c.eventCh), cap(c.eventCh)
		if l > int(float64(ecap)*0.8) {
			c.logger.Warningf("eventCh buffer is almost full [%d/%d]", l, ecap)
		}
	case <-c.stopCh:
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
	if err := c.updateTPRStatus(); err != nil {
		c.logger.Warningf("failed to update TPR status: %v", err)
	}
	c.logger.Infof("start running...")

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
				// Pod startup might take long, e.g. pulling image. It would deterministically become running or succeeded/failed later.
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

			if err := c.updateLocalBackupStatus(); err != nil {
				c.logger.Warningf("failed to update local backup service status: %v", err)
			}
			c.updateMemberStatus(running)
			if err := c.updateTPRStatus(); err != nil {
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

func isSpecEqual(s1, s2 spec.ClusterSpec) bool {
	if s1.Size != s2.Size || s1.Paused != s2.Paused || s1.Version != s2.Version {
		return false
	}
	return true
}

func (c *Cluster) startSeedMember(recoverFromBackup bool) error {
	m := &etcdutil.Member{
		Name:       etcdutil.CreateMemberName(c.cluster.Metadata.Name, c.memberCounter),
		Namespace:  c.cluster.Metadata.Namespace,
		SecurePeer: c.isSecurePeer(),
	}
	ms := etcdutil.NewMemberSet(m)
	if err := c.createPod(ms, m, "new", recoverFromBackup); err != nil {
		return fmt.Errorf("failed to create seed member (%s): %v", m.Name, err)
	}
	c.memberCounter++
	c.members = ms
	c.logger.Infof("cluster created with seed member (%s)", m.Name)
	return nil
}

func (c *Cluster) isSecurePeer() bool {
	return c.cluster.Spec.TLS != nil
}

// bootstrap creates the seed etcd member for a new cluster.
func (c *Cluster) bootstrap() error {
	return c.startSeedMember(false)
}

// recover recovers the cluster by creating a seed etcd member from a backup.
func (c *Cluster) recover() error {
	return c.startSeedMember(true)
}

func (c *Cluster) Update(cl *spec.Cluster) {
	c.send(&clusterEvent{
		typ:     eventModifyCluster,
		cluster: cl,
	})
}

func (c *Cluster) delete() {
	c.gc.CollectCluster(c.cluster.Metadata.Name, garbagecollection.NullUID)

	if c.bm == nil {
		return
	}

	if err := c.bm.cleanup(); err != nil {
		c.logger.Errorf("cluster deletion: backup manager failed to cleanup: %v", err)
	}
}

func (c *Cluster) setupServices() error {
	err := k8sutil.CreateClientService(c.config.KubeCli, c.cluster.Metadata.Name, c.cluster.Metadata.Namespace, c.cluster.AsOwner())
	if err != nil {
		return err
	}

	return k8sutil.CreatePeerService(c.config.KubeCli, c.cluster.Metadata.Name, c.cluster.Metadata.Namespace, c.cluster.AsOwner())
}

func (c *Cluster) deleteClientServiceLB() error {
	err := c.config.KubeCli.Core().Services(c.cluster.Metadata.Namespace).Delete(k8sutil.ClientServiceName(c.cluster.Metadata.Name), nil)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	err = c.config.KubeCli.Core().Services(c.cluster.Metadata.Namespace).Delete(c.cluster.Metadata.Name, nil)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) createPod(members etcdutil.MemberSet, m *etcdutil.Member, state string, needRecovery bool) error {
	token := ""
	if state == "new" {
		token = uuid.New()
	}

	pod := k8sutil.NewEtcdPod(m, members.PeerURLPairs(), c.cluster.Metadata.Name, state, token, c.cluster.Spec, c.cluster.AsOwner())
	if needRecovery {
		k8sutil.AddRecoveryToPod(pod, c.cluster.Metadata.Name, token, m, c.cluster.Spec)
	}
	_, err := c.config.KubeCli.Core().Pods(c.cluster.Metadata.Namespace).Create(pod)
	if err != nil {
		return err
	}
	return nil
}

func (c *Cluster) removePodAndService(name string) error {
	ns := c.cluster.Metadata.Namespace
	err := c.config.KubeCli.Core().Services(ns).Delete(name, nil)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}

	opts := metav1.NewDeleteOptions(podTerminationGracePeriod)
	err = c.config.KubeCli.Core().Pods(ns).Delete(name, opts)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) pollPods() (running, pending []*v1.Pod, err error) {
	podList, err := c.config.KubeCli.Core().Pods(c.cluster.Metadata.Namespace).List(k8sutil.ClusterListOpt(c.cluster.Metadata.Name))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list running pods: %v", err)
	}

	for i := range podList.Items {
		pod := &podList.Items[i]
		if len(pod.OwnerReferences) < 1 {
			c.logger.Warningf("pollPods: ignore pod %v: no owner", pod.Name)
			continue
		}
		if pod.OwnerReferences[0].UID != c.cluster.Metadata.UID {
			c.logger.Warningf("pollPods: ignore pod %v: owner (%v) is not %v",
				pod.Name, pod.OwnerReferences[0].UID, c.cluster.Metadata.UID)
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

func (c *Cluster) updateMemberStatus(pods []*v1.Pod) {
	var ready, unready []*v1.Pod
	for _, pod := range pods {
		// TODO: Change to URL struct for TLS integration
		url := fmt.Sprintf("http://%s:2379", pod.Status.PodIP)
		healthy, err := etcdutil.CheckHealth(url)
		if err != nil {
			c.logger.Warningf("health check of etcd member (%s) failed: %v", url, err)
		}
		if healthy {
			ready = append(ready, pod)
		} else {
			unready = append(unready, pod)
		}
	}
	c.status.Members.Ready = k8sutil.GetPodNames(ready)
	c.status.Members.Unready = k8sutil.GetPodNames(unready)
}

func (c *Cluster) updateTPRStatus() error {
	if reflect.DeepEqual(c.cluster.Status, c.status) {
		return nil
	}

	newCluster := c.cluster
	newCluster.Status = c.status
	newCluster, err := k8sutil.UpdateClusterTPRObject(c.config.KubeCli.Core().RESTClient(), c.cluster.Metadata.Namespace, newCluster)
	if err != nil {
		return err
	}

	c.cluster = newCluster

	return nil
}

func (c *Cluster) updateLocalBackupStatus() error {
	if c.bm == nil {
		return nil
	}

	bs, err := c.bm.getStatus()
	if err != nil {
		return err
	}
	c.status.BackupServiceStatus = backupServiceStatusToTPRBackupServiceStatu(bs)

	return nil
}

func (c *Cluster) reportFailedStatus() {
	retryInterval := 5 * time.Second

	f := func() (bool, error) {
		c.status.SetPhase(spec.ClusterPhaseFailed)
		err := c.updateTPRStatus()
		if err == nil || k8sutil.IsKubernetesResourceNotFoundError(err) {
			return true, nil
		}

		if !apierrors.IsConflict(err) {
			c.logger.Warningf("retry report status in %v: fail to update: %v", retryInterval, err)
			return false, nil
		}

		cl, err := k8sutil.GetClusterTPRObject(c.config.KubeCli.CoreV1().RESTClient(), c.cluster.Metadata.Namespace, c.cluster.Metadata.Name)
		if err != nil {
			// Update (PUT) will return conflict even if object is deleted since we have UID set in object.
			// Because it will check UID first and return something like:
			// "Precondition failed: UID in precondition: 0xc42712c0f0, UID in object meta: ".
			if k8sutil.IsKubernetesResourceNotFoundError(err) {
				return true, nil
			}
			c.logger.Warningf("retry report status in %v: fail to get latest version: %v", retryInterval, err)
			return false, nil
		}
		c.cluster = cl
		return false, nil

	}

	retryutil.Retry(retryInterval, math.MaxInt64, f)
}
