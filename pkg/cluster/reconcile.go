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
	"errors"
	"fmt"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/coreos/kube-etcd-controller/pkg/spec"
	"github.com/coreos/kube-etcd-controller/pkg/util/constants"
	"github.com/coreos/kube-etcd-controller/pkg/util/etcdutil"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
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
	log.Println("Reconciling:")
	defer log.Println("Finish reconciling")

	switch {
	case len(pods) != c.spec.Size:
		running := etcdutil.MemberSet{}
		for _, pod := range pods {
			running.Add(&etcdutil.Member{Name: pod.Name})
		}
		return c.reconcileSize(running)
	case needUpgrade(pods, c.spec):
		c.status.upgradeVersionTo(c.spec.Version)
		c.status.reportToTPR(c.kclient, c.masterHost, c.namespace, c.name)

		m := pickOneOldMember(pods, c.spec.Version)
		return c.upgradeOneMember(m)
	default:
		c.status.setVersion(c.spec.Version)
		c.status.reportToTPR(c.kclient, c.masterHost, c.namespace, c.name)

		return nil
	}
}

// reconcileSize reconciles
// - the members in the cluster view with running pods in Kubernetes.
// - the members and expect size of cluster.
//
// Definitions:
// - running pods in k8s cluster
// - members in controller knowledge
// Steps:
// 1. Remove all pods from running set that does not belong to member set.
// 2. L consist of remaining pods of runnings
// 3. If L = members, the current state matches the membership state. END.
// 4. If len(L) < len(members)/2 + 1, quorum lost. Go to recovery process.
// 5. Add one missing member. END.
func (c *Cluster) reconcileSize(running etcdutil.MemberSet) error {
	log.Infof("Running members: %s", running)
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
			log.Errorf("fail to refresh members: %v", err)
			return err
		}
	}

	log.Println("Expected membership:", c.members)

	unknownMembers := running.Diff(c.members)
	if unknownMembers.Size() > 0 {
		log.Println("Removing unexpected pods:", unknownMembers)
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
		log.Println("Disaster recovery")
		return c.disasterRecovery(L)
	}

	log.Println("Recovering one member")
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
		log.Errorf("fail to add new member (%s): %v", newMember.Name, err)
		return err
	}
	newMember.ID = resp.Member.ID
	c.members.Add(newMember)

	if err := c.createPodAndService(c.members, newMember, "existing", false); err != nil {
		log.Errorf("fail to create member (%s): %v", newMember.Name, err)
		return err
	}
	c.idCounter++
	log.Printf("added member (%s), cluster (%s)", newMember.Name, c.members.PeerURLPairs())
	return nil
}

func (c *Cluster) removeOneMember() error {
	return c.removeMember(c.members.PickOne())
}

func (c *Cluster) removeMember(toRemove *etcdutil.Member) error {
	err := removeMember(c.members.ClientURLs(), toRemove.ID)
	if err != nil {
		switch err {
		case rpctypes.ErrGRPCMemberNotFound:
			log.Infof("etcd member (%v) has been removed", toRemove.Name)
		default:
			log.Errorf("fail to remove etcd member (%v): %v", toRemove.Name, err)
			return err
		}
	}
	c.members.Remove(toRemove.Name)
	if err := c.removePodAndService(toRemove.Name); err != nil {
		return err
	}
	log.Printf("removed member (%v) with ID (%d)", toRemove.Name, toRemove.ID)
	return nil
}

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
	if c.spec.Backup == nil {
		log.Errorf("fail to do disaster recovery for cluster (%s): no backup policy has been defined.", c.name)
		return errNoBackupExist
	}
	ok := requestBackupNow(c.kclient.RESTClient.Client, k8sutil.MakeBackupHostPort(c.name))
	if ok {
		log.Info("Made a latest backup successfully")
	} else {
		// We don't return error if backupnow failed. Instead, we ask if there is previous backup.
		// If so, we can still continue. Otherwise, it's fatal error.
		exist, err := checkBackupExist(c.kclient.RESTClient.Client, k8sutil.MakeBackupHostPort(c.name))
		if err != nil {
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

func requestBackupNow(httpClient *http.Client, addr string) bool {
	resp, err := httpClient.Get(fmt.Sprintf("http://%s/backupnow", addr))
	if err != nil {
		log.Errorf("backupnow (%s) request failed: %v", addr, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Errorf("backupnow (%s): unexpected status code (%v)", addr, resp.Status)
		return false
	}
	return true
}

func checkBackupExist(httpClient *http.Client, addr string) (bool, error) {
	resp, err := httpClient.Head(fmt.Sprintf("http://%s/backupnow", addr))
	if err != nil {
		log.Errorf("check backup (%s) failed: %v", addr, err)
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Errorf("check backup (%s): unexpected status code (%v)", addr, resp.Status)
		return false, nil
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
