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
	"net/http"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/api"
)

var (
	errNoBackupExist = errors.New("No backup exist for a disaster recovery")
)

// reconcile reconciles cluster current state to desired state specified by spec.
// - it tries to reconcile the cluster to desired size.
// - if the cluster needs for upgrade, it tries to upgrade old member one by one.
func (c *Cluster) reconcile(pods []*api.Pod) error {
	c.logger.Infoln("Start reconciling")
	defer c.logger.Infoln("Finish reconciling")

	switch {
	case len(pods) != c.spec.Size:
		running := etcdutil.MemberSet{}
		for _, pod := range pods {
			m := &etcdutil.Member{Name: pod.Name}
			if c.spec.SelfHosted != nil {
				m.ClientURLs = []string{"http://" + pod.Status.PodIP + ":2379"}
				m.PeerURLs = []string{"http://" + pod.Status.PodIP + ":2380"}
			}
			running.Add(m)
		}
		return c.reconcileSize(running)
	case needUpgrade(pods, c.spec):
		c.status.upgradeVersionTo(c.spec.Version)

		m := pickOneOldMember(pods, c.spec.Version)
		return c.upgradeOneMember(m)
	default:
		c.status.setVersion(c.spec.Version)
		return nil
	}
}

// reconcileSize reconciles
// - the members in the cluster view with running pods in Kubernetes.
// - the members and expect size of cluster.
//
// Definitions:
// - running pods in k8s cluster
// - members in etcd-operator knowledge
// Steps:
// 1. Remove all pods from running set that does not belong to member set.
// 2. L consist of remaining pods of runnings
// 3. If L = members, the current state matches the membership state. END.
// 4. If len(L) < len(members)/2 + 1, quorum lost. Go to recovery process.
// 5. Add one missing member. END.
func (c *Cluster) reconcileSize(running etcdutil.MemberSet) error {
	c.logger.Infof("running members: %s", running)
	if len(c.members) == 0 {
		cfg := clientv3.Config{
			Endpoints:   running.ClientURLs(),
			DialTimeout: constants.DefaultDialTimeout,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			return err
		}
		defer etcdcli.Close()
		if err := c.updateMembers(etcdcli); err != nil {
			c.logger.Errorf("fail to refresh members: %v", err)
			return err
		}
	}

	c.logger.Infof("Expected membership: %s", c.members)

	unknownMembers := running.Diff(c.members)
	if unknownMembers.Size() > 0 {
		c.logger.Infof("Removing unexpected pods:", unknownMembers)
		for _, m := range unknownMembers {
			if err := c.removePodAndService(m.Name); err != nil {
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

	c.logger.Infof("Recovering one member")
	toRecover := c.members.Diff(L).PickOne()

	if err := c.removeMember(toRecover); err != nil {
		return err
	}
	return c.resize()
}

func (c *Cluster) resize() error {
	if c.members.Size() == c.spec.Size {
		return nil
	}

	if c.members.Size() < c.spec.Size {
		if c.spec.SelfHosted != nil {
			return c.addOneSelfHostedMember()
		}

		return c.addOneMember()
	}

	return c.removeOneMember()
}

func (c *Cluster) addOneMember() error {
	cfg := clientv3.Config{
		Endpoints:   c.members.ClientURLs(),
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	newMemberName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	newMember := &etcdutil.Member{Name: newMemberName}
	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberAdd(ctx, []string{newMember.PeerAddr()})
	if err != nil {
		c.logger.Errorf("fail to add new member (%s): %v", newMember.Name, err)
		return err
	}
	newMember.ID = resp.Member.ID
	c.members.Add(newMember)

	if err := c.createPodAndService(c.members, newMember, "existing", false); err != nil {
		c.logger.Errorf("fail to create member (%s): %v", newMember.Name, err)
		return err
	}
	c.idCounter++
	c.logger.Infof("added member (%s)", newMember.Name)
	return nil
}

func (c *Cluster) removeOneMember() error {
	return c.removeMember(c.members.PickOne())
}

func (c *Cluster) removeMember(toRemove *etcdutil.Member) error {
	err := removeMember(c.members.ClientURLs(), toRemove.ID)
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
	if err := c.removePodAndService(toRemove.Name); err != nil {
		return err
	}
	c.logger.Infof("removed member (%v) with ID (%d)", toRemove.Name, toRemove.ID)
	return nil
}

// TODO: move removeMember to etcdutil
func removeMember(clientURLs []string, id uint64) error {
	cfg := clientv3.Config{
		Endpoints:   clientURLs,
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	if _, err = etcdcli.Cluster.MemberRemove(ctx, id); err != nil {
		return err
	}
	return nil
}

func (c *Cluster) disasterRecovery(left etcdutil.MemberSet) error {
	if c.spec.SelfHosted != nil {
		return errors.New("self-hosted cluster cannot be recovered from disaster")
	}

	if c.spec.Backup == nil {
		c.logger.Errorf("fail to do disaster recovery: no backup policy has been defined.")
		return errNoBackupExist
	}

	backupNow := true
	if len(left) > 0 {
		c.logger.Infof("pods are still running (%v). Will try to make a latest backup from one of them.", left)
		err := RequestBackupNow(c.kclient.RESTClient.Client, k8sutil.MakeBackupHostPort(c.name))
		if err != nil {
			backupNow = false
			c.logger.Errorln(err)
		}
	}
	if backupNow {
		c.logger.Info("made a latest backup")
	} else {
		// We don't return error if backupnow failed. Instead, we ask if there is previous backup.
		// If so, we can still continue. Otherwise, it's fatal error.
		exist, err := checkBackupExist(c.kclient.RESTClient.Client, k8sutil.MakeBackupHostPort(c.name))
		if err != nil {
			c.logger.Errorln(err)
			return err
		}
		if !exist {
			return errNoBackupExist
		}
	}

	for _, m := range left {
		err := c.removePodAndService(m.Name)
		if err != nil {
			return err
		}
	}
	c.members = nil
	return c.restoreSeedMember()
}

// TODO: make this private
func RequestBackupNow(httpClient *http.Client, addr string) error {
	resp, err := httpClient.Get(fmt.Sprintf("http://%s/backupnow", addr))
	if err != nil {
		return fmt.Errorf("backupnow (%s) request failed: %v", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("backupnow (%s): unexpected status code (%v)", addr, resp.Status)
	}
	return nil
}

func checkBackupExist(httpClient *http.Client, addr string) (bool, error) {
	resp, err := httpClient.Head(fmt.Sprintf("http://%s/backup", addr))
	if err != nil {
		return false, fmt.Errorf("check backup (%s) failed: %v", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("check backup (%s) failed: unexpected status code (%v)", addr, resp.Status)
	}
	return true, nil
}

func needUpgrade(pods []*api.Pod, cs *spec.ClusterSpec) bool {
	return len(pods) == cs.Size && pickOneOldMember(pods, cs.Version) != nil
}

func pickOneOldMember(pods []*api.Pod, newVersion string) *etcdutil.Member {
	for _, pod := range pods {
		if k8sutil.GetEtcdVersion(pod) == newVersion {
			continue
		}
		return &etcdutil.Member{Name: pod.Name}
	}
	return nil
}
