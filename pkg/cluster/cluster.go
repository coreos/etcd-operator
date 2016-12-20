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

	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/cluster/backupmanager"
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

type Config struct {
	Name          string
	Namespace     string
	PVProvisioner string
	s3config.S3Context
	KubeCli *unversioned.Client
}

type Cluster struct {
	logger *logrus.Entry

	Config

	status *Status

	spec *spec.ClusterSpec

	idCounter int
	eventCh   chan *clusterEvent
	stopCh    chan struct{}

	// members repsersents the members in the etcd cluster.
	// the name of the member is the the name of the pod the member
	// process runs in.
	members etcdutil.MemberSet

	bm        backupmanager.BackupManager
	backupDir string
}

func New(c Config, s *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup) (*Cluster, error) {
	return new(c, s, stopC, wg, true)
}

func Restore(c Config, s *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup) (*Cluster, error) {
	return new(c, s, stopC, wg, false)
}

func new(config Config, s *spec.ClusterSpec, stopC <-chan struct{}, wg *sync.WaitGroup, isNewCluster bool) (*Cluster, error) {
	if len(s.Version) == 0 {
		// TODO: set version in spec in apiserver
		s.Version = defaultVersion
	}

	err := s.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid cluster spec: %v", err)
	}

	var bm backupmanager.BackupManager
	if backup := s.Backup; backup != nil {
		switch backup.StorageType {
		case spec.BackupStorageTypePersistentVolume, spec.BackupStorageTypeDefault:
			bm = backupmanager.NewPVBackupManager(config.KubeCli, config.Name, config.Namespace, config.PVProvisioner, *backup)
		case spec.BackupStorageTypeS3:
			bm, err = backupmanager.NewS3BackupManager(config.S3Context, config.KubeCli, config.Name, config.Namespace)
			if err != nil {
				return nil, err
			}
		}
	}

	c := &Cluster{
		logger:  logrus.WithField("pkg", "cluster").WithField("cluster-name", config.Name),
		Config:  config,
		spec:    s,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
		status:  &Status{},
		bm:      bm,
	}

	if isNewCluster {
		if c.spec.Backup != nil {
			if err := c.prepareBackupAndRestore(); err != nil {
				return nil, err
			}
		}

		if c.spec.Restore == nil {
			// Note: For restore case, we will go through reconcile loop and disaster recovery.
			if err := c.prepareSeedMember(); err != nil {
				return nil, err
			}
		}

		if err := c.createClientServiceLB(); err != nil {
			return nil, fmt.Errorf("fail to create client service LB: %v", err)
		}
	}

	wg.Add(1)
	go c.run(stopC, wg)

	return c, nil
}

func (c *Cluster) prepareBackupAndRestore() error {
	backup, restore := c.spec.Backup, c.spec.Restore

	if restore == nil {
		if err := c.bm.Setup(); err != nil {
			return err
		}
	} else {
		c.logger.Infof("restoring cluster from existing backup (%s)", restore.BackupClusterName)
		if restore.BackupClusterName == c.Name {
			// TODO: check the existence of the backup. error out if the backup does not exist.
			c.logger.Infof("recreating the cluster: using the existing backup")
		} else {
			c.logger.Infof("restoring the previous cluster (%s): cloning data from existing backup", restore.BackupClusterName)
			if err := c.bm.Clone(restore.BackupClusterName); err != nil {
				return err
			}
		}
	}

	podSpec, err := k8sutil.MakeBackupPodSpec(c.Name, backup)
	if err != nil {
		return err
	}
	podSpec = c.bm.PodSpecWithStorage(podSpec)
	err = k8sutil.CreateBackupReplicaSetAndService(c.KubeCli, c.Name, c.Namespace, *podSpec)
	if err != nil {
		return fmt.Errorf("failed to create backup replica set and service: %v", err)
	}
	c.logger.Info("backup replica set and service created")
	return nil
}

func (c *Cluster) deleteBackup() {
	err := k8sutil.DeleteBackupReplicaSetAndService(c.KubeCli, c.Name, c.Namespace)
	if err != nil {
		c.logger.Errorf("cluster deletion: fail to delete backup ReplicaSet: %v", err)
	}
	c.logger.Infof("backup replica set and service deleted")

	err = c.bm.CleanupBackups()
	if err != nil {
		c.logger.Errorf("cluster deletion: fail to cleanup backups: %v", err)
	}
}

func (c *Cluster) prepareSeedMember() error {
	var err error
	if c.spec.SelfHosted != nil {
		if len(c.spec.SelfHosted.BootMemberClientEndpoint) == 0 {
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
	etcdName := fmt.Sprintf("%s-%04d", c.Name, c.idCounter)
	return &etcdutil.Member{Name: etcdName}
}

func (c *Cluster) startSeedMember(recoverFromBackup bool) error {
	m := c.makeSeedMember()
	if err := c.createPodAndService(etcdutil.NewMemberSet(m), m, "new", recoverFromBackup); err != nil {
		c.logger.Errorf("failed to create seed member (%s): %v", m.Name, err)
		return err
	}
	c.idCounter++
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
			"etcd_cluster": c.Name,
			"app":          "etcd",
		}),
	}

	pods, err := c.KubeCli.Pods(c.Namespace).List(option)
	if err != nil {
		c.logger.Errorf("cluster deletion: cannot delete any pod due to failure to list: %v", err)
	} else {
		for i := range pods.Items {
			if err := c.removePodAndService(pods.Items[i].Name); err != nil {
				c.logger.Errorf("cluster deletion: fail to delete (%s)'s pod and svc: %v", pods.Items[i].Name, err)
			}
		}
	}

	err = c.deleteClientServiceLB()
	if err != nil {
		c.logger.Errorf("cluster deletion: fail to delete client service LB: %v", err)
	}

	if c.spec.Backup != nil {
		c.deleteBackup()
	}
}

func (c *Cluster) createClientServiceLB() error {
	if _, err := k8sutil.CreateEtcdService(c.KubeCli, c.Name, c.Namespace); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) deleteClientServiceLB() error {
	err := c.KubeCli.Services(c.Namespace).Delete(c.Name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) createPodAndService(members etcdutil.MemberSet, m *etcdutil.Member, state string, needRecovery bool) error {
	// TODO: remove garbage service. Because we will fail after service created before pods created.
	if _, err := k8sutil.CreateEtcdMemberService(c.KubeCli, m.Name, c.Name, c.Namespace); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	token := ""
	if state == "new" {
		token = uuid.New()
	}
	pod := k8sutil.MakeEtcdPod(m, members.PeerURLPairs(), c.Name, state, token, c.spec)
	if needRecovery {
		k8sutil.AddRecoveryToPod(pod, c.Name, m.Name, token, c.spec)
	}
	_, err := c.KubeCli.Pods(c.Namespace).Create(pod)
	return err
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

func (c *Cluster) pollPods() ([]*k8sapi.Pod, []*k8sapi.Pod, error) {
	podList, err := c.KubeCli.Pods(c.Namespace).List(k8sutil.EtcdPodListOpt(c.Name))
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
