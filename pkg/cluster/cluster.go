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
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/client"
	"github.com/coreos/etcd-operator/pkg/debug"
	"github.com/coreos/etcd-operator/pkg/garbagecollection"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	"github.com/Sirupsen/logrus"
	"github.com/pborman/uuid"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
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
	cluster *api.EtcdCluster
}

type Config struct {
	PVProvisioner  string
	ServiceAccount string

	KubeCli   kubernetes.Interface
	EtcdCRCli client.EtcdClusterCR
}

type Cluster struct {
	logger *logrus.Entry
	// debug logger for self hosted cluster
	debugLogger *debug.DebugLogger

	config Config

	cluster *api.EtcdCluster

	// in memory state of the cluster
	// status is the source of truth after Cluster struct is materialized.
	status        api.ClusterStatus
	memberCounter int

	eventCh chan *clusterEvent
	stopCh  chan struct{}

	// members repsersents the members in the etcd cluster.
	// the name of the member is the the name of the pod the member
	// process runs in.
	members etcdutil.MemberSet

	bm *backupManager

	tlsConfig *tls.Config

	gc *garbagecollection.GC

	eventsCli corev1.EventInterface
}

func New(config Config, cl *api.EtcdCluster) *Cluster {
	lg := logrus.WithField("pkg", "cluster").WithField("cluster-name", cl.Name)
	var debugLogger *debug.DebugLogger
	if cl.Spec.SelfHosted != nil {
		debugLogger = debug.New(cl.Name)
	}

	c := &Cluster{
		logger:      lg,
		debugLogger: debugLogger,
		config:      config,
		cluster:     cl,
		eventCh:     make(chan *clusterEvent, 100),
		stopCh:      make(chan struct{}),
		status:      *(cl.Status.DeepCopy()),
		gc:          garbagecollection.New(config.KubeCli, cl.Namespace),
		eventsCli:   config.KubeCli.Core().Events(cl.Namespace),
	}

	go func() {
		if err := c.setup(); err != nil {
			c.logger.Errorf("cluster failed to setup: %v", err)
			if c.status.Phase != api.ClusterPhaseFailed {
				c.status.SetReason(err.Error())
				c.status.SetPhase(api.ClusterPhaseFailed)
				if err := c.updateCRStatus(); err != nil {
					c.logger.Errorf("failed to update cluster phase (%v): %v", api.ClusterPhaseFailed, err)
				}
			}
			return
		}
		c.run()
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
	case api.ClusterPhaseNone:
		shouldCreateCluster = true
	case api.ClusterPhaseCreating:
		return errCreatedCluster
	case api.ClusterPhaseRunning:
		shouldCreateCluster = false

	default:
		return fmt.Errorf("unexpected cluster phase: %s", c.status.Phase)
	}

	if c.isSecureClient() {
		d, err := k8sutil.GetTLSDataFromSecret(c.config.KubeCli, c.cluster.Namespace, c.cluster.Spec.TLS.Static.OperatorSecret)
		if err != nil {
			return err
		}
		c.tlsConfig, err = etcdutil.NewTLSConfig(d.CertData, d.KeyData, d.CAData)
		if err != nil {
			return err
		}
	}

	if c.cluster.Spec.Backup != nil {
		c.bm, err = newBackupManager(c.config, c.cluster, c.logger)
		if err != nil {
			return err
		}
		if !shouldCreateCluster {
			err := c.bm.upgradeIfNeeded()
			if err != nil {
				return err
			}
		}
	}

	if shouldCreateCluster {
		return c.create()
	}
	return nil
}

func (c *Cluster) create() error {
	c.status.SetPhase(api.ClusterPhaseCreating)

	if err := c.updateCRStatus(); err != nil {
		return fmt.Errorf("cluster create: failed to update cluster phase (%v): %v", api.ClusterPhaseCreating, err)
	}
	c.logClusterCreation()

	c.gc.CollectCluster(c.cluster.Name, c.cluster.UID)

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

func (c *Cluster) run() {

	defer func() {
		c.logger.Infof("deleting the failed cluster")
		c.reportFailedStatus()
		c.delete()
	}()

	c.status.SetPhase(api.ClusterPhaseRunning)
	if err := c.updateCRStatus(); err != nil {
		c.logger.Warningf("update initial CR status failed: %v", err)
	}
	c.logger.Infof("start running...")

	var rerr error
	for {
		select {
		case event := <-c.eventCh:
			switch event.typ {
			case eventModifyCluster:
				if isSpecEqual(event.cluster.Spec, c.cluster.Spec) {
					break
				}
				// TODO: we can't handle another upgrade while an upgrade is in progress
				c.logSpecUpdate(event.cluster.Spec)

				ob, nb := c.cluster.Spec.Backup, event.cluster.Spec.Backup
				c.cluster = event.cluster

				if !isBackupPolicyEqual(ob, nb) {
					err := c.updateBackupPolicy(ob, nb)
					if err != nil {
						c.logger.Errorf("failed to update backup policy: %v", err)
						c.status.SetReason(err.Error())
						return
					}
				}

			case eventDeleteCluster:
				c.logger.Infof("cluster is deleted by the user")
				return
			default:
				panic("unknown event type" + event.typ)
			}

		case <-time.After(reconcileInterval):
			start := time.Now()

			if c.cluster.Spec.Paused {
				c.status.PauseControl()
				c.logger.Infof("control is paused, skipping reconciliation")
				continue
			} else {
				c.status.Control()
			}

			running, pending, err := c.pollPods()
			if err != nil {
				c.logger.Errorf("fail to poll pods: %v", err)
				reconcileFailed.WithLabelValues("failed to poll pods").Inc()
				continue
			}

			if len(pending) > 0 {
				// Pod startup might take long, e.g. pulling image. It would deterministically become running or succeeded/failed later.
				c.logger.Infof("skip reconciliation: running (%v), pending (%v)", k8sutil.GetPodNames(running), k8sutil.GetPodNames(pending))
				reconcileFailed.WithLabelValues("not all pods are running").Inc()
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
				rerr = c.updateMembers(podsToMemberSet(running, c.isSecureClient()))
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
			c.updateMemberStatus(c.members)
			if err := c.updateCRStatus(); err != nil {
				c.logger.Warningf("periodic update CR status failed: %v", err)
			}

			reconcileHistogram.WithLabelValues(c.name()).Observe(time.Since(start).Seconds())
		}

		if rerr != nil {
			reconcileFailed.WithLabelValues(rerr.Error()).Inc()
		}

		if isFatalError(rerr) {
			c.status.SetReason(rerr.Error())
			c.logger.Errorf("cluster failed: %v", rerr)
			return
		}
	}
}

func (c *Cluster) updateBackupPolicy(ob, nb *api.BackupPolicy) error {
	var err error
	switch {
	case ob == nil && nb != nil:
		c.bm, err = newBackupManager(c.config, c.cluster, c.logger)
		if err != nil {
			return err
		}
		return c.bm.setup()
	case ob != nil && nb == nil:
		c.bm.deleteBackupSidecar()
		c.bm = nil
	case ob != nil && nb != nil:
		return c.bm.updateSidecar(c.cluster)
	default:
		panic("unexpected backup spec comparison")
	}
	return nil
}

func isSpecEqual(s1, s2 api.ClusterSpec) bool {
	if s1.Size != s2.Size || s1.Paused != s2.Paused || s1.Version != s2.Version {
		return false
	}
	return isBackupPolicyEqual(s1.Backup, s2.Backup)
}

func isBackupPolicyEqual(b1, b2 *api.BackupPolicy) bool {
	return reflect.DeepEqual(b1, b2)
}

func (c *Cluster) startSeedMember(recoverFromBackup bool) error {
	m := &etcdutil.Member{
		Name:         etcdutil.CreateMemberName(c.cluster.Name, c.memberCounter),
		Namespace:    c.cluster.Namespace,
		SecurePeer:   c.isSecurePeer(),
		SecureClient: c.isSecureClient(),
	}
	ms := etcdutil.NewMemberSet(m)
	if err := c.createPod(ms, m, "new", recoverFromBackup); err != nil {
		return fmt.Errorf("failed to create seed member (%s): %v", m.Name, err)
	}
	c.memberCounter++
	c.members = ms
	c.logger.Infof("cluster created with seed member (%s)", m.Name)
	_, err := c.eventsCli.Create(k8sutil.NewMemberAddEvent(m.Name, c.cluster))
	if err != nil {
		c.logger.Errorf("failed to create new member add event: %v", err)
	}

	return nil
}

func (c *Cluster) isSecurePeer() bool {
	return c.cluster.Spec.TLS.IsSecurePeer()
}

func (c *Cluster) isSecureClient() bool {
	return c.cluster.Spec.TLS.IsSecureClient()
}

// bootstrap creates the seed etcd member for a new cluster.
func (c *Cluster) bootstrap() error {
	return c.startSeedMember(false)
}

// recover recovers the cluster by creating a seed etcd member from a backup.
func (c *Cluster) recover() error {
	return c.startSeedMember(true)
}

func (c *Cluster) Update(cl *api.EtcdCluster) {
	c.send(&clusterEvent{
		typ:     eventModifyCluster,
		cluster: cl,
	})
}

func (c *Cluster) delete() {
	c.gc.CollectCluster(c.cluster.Name, garbagecollection.NullUID)

	if c.bm == nil {
		return
	}

	if err := c.bm.cleanup(); err != nil {
		c.logger.Errorf("cluster deletion: backup manager failed to cleanup: %v", err)
	}
}

func (c *Cluster) setupServices() error {
	err := k8sutil.CreateClientService(c.config.KubeCli, c.cluster.Name, c.cluster.Namespace, c.cluster.AsOwner())
	if err != nil {
		return err
	}

	return k8sutil.CreatePeerService(c.config.KubeCli, c.cluster.Name, c.cluster.Namespace, c.cluster.AsOwner())
}

func (c *Cluster) createPod(members etcdutil.MemberSet, m *etcdutil.Member, state string, needRecovery bool) error {
	token := ""
	if state == "new" {
		token = uuid.New()
	}

	pod := k8sutil.NewEtcdPod(m, members.PeerURLPairs(), c.cluster.Name, state, token, c.cluster.Spec, c.cluster.AsOwner())
	if needRecovery {
		k8sutil.AddRecoveryToPod(pod, c.cluster.Name, token, m, c.cluster.Spec)
	}
	_, err := c.config.KubeCli.Core().Pods(c.cluster.Namespace).Create(pod)
	return err
}

func (c *Cluster) removePod(name string) error {
	ns := c.cluster.Namespace
	opts := metav1.NewDeleteOptions(podTerminationGracePeriod)
	err := c.config.KubeCli.Core().Pods(ns).Delete(name, opts)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
		if c.isDebugLoggerEnabled() {
			c.debugLogger.LogMessage(fmt.Sprintf("pod (%s) not found while trying to delete it", name))
		}
	}
	if c.isDebugLoggerEnabled() {
		c.debugLogger.LogPodDeletion(name)
	}
	return nil
}

func (c *Cluster) pollPods() (running, pending []*v1.Pod, err error) {
	podList, err := c.config.KubeCli.Core().Pods(c.cluster.Namespace).List(k8sutil.ClusterListOpt(c.cluster.Name))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list running pods: %v", err)
	}

	for i := range podList.Items {
		pod := &podList.Items[i]
		if len(pod.OwnerReferences) < 1 {
			c.logger.Warningf("pollPods: ignore pod %v: no owner", pod.Name)
			continue
		}
		if pod.OwnerReferences[0].UID != c.cluster.UID {
			c.logger.Warningf("pollPods: ignore pod %v: owner (%v) is not %v",
				pod.Name, pod.OwnerReferences[0].UID, c.cluster.UID)
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

func (c *Cluster) updateMemberStatus(members etcdutil.MemberSet) {
	var ready, unready []string
	for _, m := range members {
		url := m.ClientURL()
		healthy, err := etcdutil.CheckHealth(url, c.tlsConfig)
		if err != nil {
			c.logger.Warningf("health check of etcd member (%s) failed: %v", url, err)
		}
		if healthy {
			ready = append(ready, m.Name)
		} else {
			unready = append(unready, m.Name)
		}
	}
	c.status.Members.Ready = ready
	c.status.Members.Unready = unready
}

func (c *Cluster) updateCRStatus() error {
	if reflect.DeepEqual(c.cluster.Status, c.status) {
		return nil
	}

	newCluster := c.cluster
	newCluster.Status = c.status
	newCluster, err := c.config.EtcdCRCli.Update(context.TODO(), c.cluster)
	if err != nil {
		return fmt.Errorf("failed to update CR status: %v", err)
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
		c.status.SetPhase(api.ClusterPhaseFailed)
		err := c.updateCRStatus()
		if err == nil || k8sutil.IsKubernetesResourceNotFoundError(err) {
			return true, nil
		}

		if !apierrors.IsConflict(err) {
			c.logger.Warningf("retry report status in %v: fail to update: %v", retryInterval, err)
			return false, nil
		}

		cl, err := c.config.EtcdCRCli.Get(context.TODO(), c.cluster.Namespace, c.cluster.Name)
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

func (c *Cluster) name() string {
	return c.cluster.GetName()
}

func (c *Cluster) logClusterCreation() {
	specBytes, err := json.MarshalIndent(c.cluster.Spec, "", "    ")
	if err != nil {
		c.logger.Errorf("failed to marshal cluster spec: %v", err)
	}

	c.logger.Info("creating cluster with Spec:")
	for _, m := range strings.Split(string(specBytes), "\n") {
		c.logger.Info(m)
	}
}

func (c *Cluster) logSpecUpdate(newSpec api.ClusterSpec) {
	oldSpecBytes, err := json.MarshalIndent(c.cluster.Spec, "", "    ")
	if err != nil {
		c.logger.Errorf("failed to marshal cluster spec: %v", err)
	}
	newSpecBytes, err := json.MarshalIndent(newSpec, "", "    ")
	if err != nil {
		c.logger.Errorf("failed to marshal cluster spec: %v", err)
	}

	c.logger.Infof("spec update: Old Spec:")
	for _, m := range strings.Split(string(oldSpecBytes), "\n") {
		c.logger.Info(m)
	}

	c.logger.Infof("New Spec:")
	for _, m := range strings.Split(string(newSpecBytes), "\n") {
		c.logger.Info(m)
	}

	if c.isDebugLoggerEnabled() {
		c.debugLogger.LogClusterSpecUpdate(string(oldSpecBytes), string(newSpecBytes))
	}
}

func (c *Cluster) isDebugLoggerEnabled() bool {
	if c.cluster.Spec.SelfHosted != nil && c.debugLogger != nil {
		return true
	}
	return false
}
