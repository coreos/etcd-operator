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

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"golang.org/x/net/context"
	"k8s.io/api/core/v1"
)

// reconcile reconciles cluster current state to desired state specified by spec.
// - it tries to reconcile the cluster to desired size.
// - if the cluster needs for upgrade, it tries to upgrade old member one by one.
func (c *Cluster) reconcile(pods []*v1.Pod, pvcs []*v1.PersistentVolumeClaim) error {
	c.logger.Infoln("Start reconciling")
	defer c.logger.Infoln("Finish reconciling")

	defer func() {
		c.status.Size = c.members.Size()
	}()

	sp := c.cluster.Spec
	running := podsToMemberSet(pods, c.isSecureClient())
	if !running.IsEqual(c.members) || c.members.Size() != sp.Size {
		return c.reconcileMembers(running)
	}

	if err := c.reconcilePVCs(pvcs); err != nil {
		return err
	}

	if needUpgrade(pods, sp) {
		c.status.UpgradeVersionTo(sp.Version)

		m := pickOneOldMember(pods, sp.Version)
		return c.upgradeOneMember(m.Name)
	}

	c.status.SetVersion(sp.Version)
	c.status.SetReadyCondition()

	return nil
}

// reconcileMembers reconciles
// - running pods on k8s and cluster membership
// - cluster membership and expected size of etcd cluster
// Steps:
// 1. Remove all pods from running set that does not belong to member set.
// 2. L consist of remaining pods of runnings
// 3. If L = members, the current state matches the membership state. END.
// 4. If len(L) < len(members)/2 + 1, quorum lost. Go to recovery process.
// 5. Add one missing member. END.
func (c *Cluster) reconcileMembers(running etcdutil.MemberSet) error {
	c.logger.Infof("running members: %s", running)
	c.logger.Infof("cluster membership: %s", c.members)

	unknownMembers := running.Diff(c.members)
	if unknownMembers.Size() > 0 {
		c.logger.Infof("removing unexpected pods: %v", unknownMembers)
		for _, m := range unknownMembers {
			if err := c.removePod(m.Name); err != nil {
				return err
			}
		}
	}
	L := running.Diff(unknownMembers)

	if L.Size() == c.members.Size() {
		return c.resize()
	}

	if L.Size() < c.members.Size()/2+1 {
		c.logger.Infof("Disaster recovery")
		return c.disasterRecovery(L)
	}

	c.logger.Infof("removing one dead member")
	// remove dead members that doesn't have any running pods before doing resizing.
	return c.removeDeadMember(c.members.Diff(L).PickOne())
}

// reconcilePVCs reconciles PVCs with current cluster members removing old PVCs
func (c *Cluster) reconcilePVCs(pvcs []*v1.PersistentVolumeClaim) error {
	oldPVCs := []string{}
	for _, pvc := range pvcs {
		memberName := etcdutil.MemberNameFromPVCName(pvc.Name)
		if _, ok := c.members[memberName]; !ok {
			oldPVCs = append(oldPVCs, pvc.Name)
		}
	}

	for _, oldPVC := range oldPVCs {
		c.logger.Infof("removing old pvc: %s", oldPVC)
		if err := c.removePVC(oldPVC); err != nil {
			return err
		}
	}

	return nil
}

func (c *Cluster) resize() error {
	if c.members.Size() == c.cluster.Spec.Size {
		return nil
	}

	if c.members.Size() < c.cluster.Spec.Size {
		if c.cluster.Spec.SelfHosted != nil {
			return c.addOneSelfHostedMember()
		}

		return c.addOneMember()
	}

	return c.removeOneMember()
}

func (c *Cluster) addOneMember() error {
	c.status.AppendScalingUpCondition(c.members.Size(), c.cluster.Spec.Size)

	cfg := clientv3.Config{
		Endpoints:   c.members.ClientURLs(),
		DialTimeout: constants.DefaultDialTimeout,
		TLS:         c.tlsConfig,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return fmt.Errorf("add one member failed: creating etcd client failed %v", err)
	}
	defer etcdcli.Close()

	newMember := c.newMember(c.memberCounter)
	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberAdd(ctx, []string{newMember.PeerURL()})
	if err != nil {
		return fmt.Errorf("fail to add new member (%s): %v", newMember.Name, err)
	}
	newMember.ID = resp.Member.ID
	c.members.Add(newMember)

	if c.UsePodPV() {
		if err := c.createPVC(newMember); err != nil {
			return fmt.Errorf("failed to create persistent volume claim for member's pod (%s): %v", newMember.Name, err)
		}
	}
	if err := c.createPod(c.members, newMember, "existing", false, c.UsePodPV()); err != nil {
		return fmt.Errorf("fail to create member's pod (%s): %v", newMember.Name, err)
	}
	c.memberCounter++
	c.logger.Infof("added member (%s)", newMember.Name)
	return nil
}

func (c *Cluster) removeOneMember() error {
	c.status.AppendScalingDownCondition(c.members.Size(), c.cluster.Spec.Size)

	return c.removeMember(c.members.PickOne())
}

func (c *Cluster) removeDeadMember(toRemove *etcdutil.Member) error {
	if c.cluster.Spec.SelfHosted != nil {
		selectedNodes, err := c.selectSchedulableNodes()
		if err != nil {
			return err
		}

		// If we do not have enough master nodes, we should simply wait for the old node
		// to be online again.
		//
		// Removing the etcd member will not help us to recover the cluster to the desired
		// number of members, since we do not have enough master nodes for self hosted etcd.
		//
		// Instead, there is a large chance that the old node is taken offline for maintenance
		// or experiencing temporary network partition. When it comes back, it will recover itself
		// since we persist data for self hosted case.

		if nodeNum := len(selectedNodes); nodeNum < c.cluster.Spec.Size {
			c.logger.Warningf("ignored removing failed member (%s). Not enough master nodes (%v) to recover, want at least %d", toRemove.Name, selectedNodes, c.cluster.Spec.Size)
			c.logger.Infof("waiting for the failed master node to recover, or more master nodes")
			return nil
		}
	}

	c.logger.Infof("removing dead member %q", toRemove.Name)
	c.status.AppendRemovingDeadMember(toRemove.Name)

	return c.removeMember(toRemove)
}

func (c *Cluster) removeMember(toRemove *etcdutil.Member) error {
	err := etcdutil.RemoveMember(c.members.ClientURLs(), c.tlsConfig, toRemove.ID)
	if err != nil {
		switch err {
		case rpctypes.ErrMemberNotFound:
			c.logger.Infof("etcd member (%v) has been removed", toRemove.Name)
		default:
			c.logger.Errorf("fail to remove etcd member (%v): %v", toRemove.Name, err)
			return err
		}
	}
	c.members.Remove(toRemove.Name)
	if err := c.removePod(toRemove.Name); err != nil {
		return err
	}
	c.logger.Infof("removed member (%v) with ID (%d)", toRemove.Name, toRemove.ID)
	return nil
}

func (c *Cluster) disasterRecovery(left etcdutil.MemberSet) error {
	c.status.AppendRecoveringCondition()

	if c.cluster.Spec.SelfHosted != nil {
		return errors.New("self-hosted cluster cannot be recovered from disaster")
	}

	if c.cluster.Spec.Backup == nil {
		return newFatalError("fail to do disaster recovery: no backup policy has been defined")
	}

	backupNow := false
	if len(left) > 0 {
		c.logger.Infof("pods are still running (%v). Will try to make a latest backup from one of them.", left)
		err := c.bm.requestBackup()
		if err != nil {
			c.logger.Errorln(err)
		} else {
			backupNow = true
		}
	}
	if backupNow {
		c.logger.Info("made a latest backup")
	} else {
		// We don't return error if backupnow failed. Instead, we ask if there is previous backup.
		// If so, we can still continue. Otherwise, it's fatal error.
		exist, err := c.bm.checkBackupExist(c.cluster.Spec.Version)
		if err != nil {
			c.logger.Errorln(err)
			return err
		}
		if !exist {
			return newFatalError("no backup exist for disaster recovery")
		}
	}

	for _, m := range left {
		err := c.removePod(m.Name)
		if err != nil {
			return err
		}
	}
	return c.recover()
}

func needUpgrade(pods []*v1.Pod, cs spec.ClusterSpec) bool {
	return len(pods) == cs.Size && pickOneOldMember(pods, cs.Version) != nil
}

func pickOneOldMember(pods []*v1.Pod, newVersion string) *etcdutil.Member {
	for _, pod := range pods {
		if k8sutil.GetEtcdVersion(pod) == newVersion {
			continue
		}
		return &etcdutil.Member{Name: pod.Name, Namespace: pod.Namespace}
	}
	return nil
}
