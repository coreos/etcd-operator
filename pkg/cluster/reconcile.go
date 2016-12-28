package cluster

import (
	"github.com/GregoryIan/operator/pkg/cluster/member"
	"github.com/GregoryIan/operator/pkg/util"
	"github.com/GregoryIan/operator/pkg/util/pdutil"
	"github.com/juju/errors"
	"github.com/ngaut/log"

	"k8s.io/kubernetes/pkg/api"
)

func (c *Cluster) reconcile(pods []*api.Pod, tp member.MemberType) error {
	log.Infof("Start reconciling %s", tp)
	defer log.Infof("Finish reconciling %s", tp)

	switch {
	// determine whether the length of running pod and spec is equal?
	// if not, reconcile the size;
	case c.members[tp].NotEqualPodSize(len(pods)):
		running := member.GetEmptyMemberSet(c.KubeCli, c.Name, c.Namespace, tp)
		running.RestoreFromPods(pods)
		return c.reconcileSize(running, tp)
	case c.members[tp].NeedUpgrade(pods):
		m := c.members[tp].PickOneOldMember(pods)
		return c.upgradeOneMember(m, tp)
	default:
		return nil
	}
}

func (c *Cluster) reconcileSize(running member.MemberSet, tp member.MemberType) error {
	// if members' size == 0, update members from pd; it's pd views
	if c.members[tp].Size() == 0 {
		pdcli := pdutil.New(running.(*member.PDMemberSet).ClientURLs())
		if err := c.members[tp].UpdateMembers(pdcli); err != nil {
			log.Errorf("fail to refresh members: %v", err)
			return err
		}
	}

	// find the pods that not in members, then delete them
	unknownMembers := running.Diff(c.members[tp])
	if unknownMembers.Size() > 0 {
		log.Infof("Removing unexpected pods:", unknownMembers)
		for _, m := range unknownMembers.Members() {
			if err := c.removePod(m.Name); err != nil {
				return err
			}
		}
	}
	// L is the running pod that also in members
	L := running.Diff(unknownMembers)
	if L.Size() == c.members[tp].Size() {
		return c.resize(tp)
	}

	// determine whether the cluster is not consistent
	if member.IsNonConsistent(L.Size(), c.members[tp].Size(), tp) {
		log.Fatal("Disaster recovery")
	}

	// deal the condition that some pod is dead or removed, but membership isn't deleted
	// len(members) > len(L)
	log.Infof("Recovering one member")
	toRecover := c.members[tp].Diff(L).PickOne()
	if err := c.removeMember(toRecover.Name, tp); err != nil {
		return err
	}
	return c.resize(tp)
}

func (c *Cluster) resize(tp member.MemberType) error {
	size := member.GetSpecSize(c.spec, tp)
	if c.members[tp].Size() == size {
		return nil
	}
	if c.members[tp].Size() < size {
		return c.members[tp].NewOneMember()
	}

	return c.removeOneMember(tp)
}

func (c *Cluster) removeOneMember(tp member.MemberType) error {
	m := c.members[tp].PickOne()
	return c.removeMember(m.Name, tp)
}

func (c *Cluster) removeMember(name string, tp member.MemberType) error {
	ms, _ := c.members[member.PD].(*member.PDMemberSet)
	err := member.RemoveMember(ms.ClientURLs(), name, tp)
	if err != nil {
		return errors.Trace(err)
	}
	c.members[tp].RemoveMemberShip(name)
	if err := c.removePod(name); err != nil {
		return errors.Trace(err)
	}
	log.Infof("removed member (%v)", name)
	return nil
}

func (c *Cluster) removePod(name string) error {
	err := c.KubeCli.Pods(c.Namespace).Delete(name, api.NewDeleteOptions(0))
	if err != nil {
		if !util.IsKubernetesResourceNotFoundError(err) {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *Cluster) upgradeOneMember(m *member.Member, tp member.MemberType) error {
	pod, err := c.KubeCli.Pods(c.Namespace).Get(m.Name)
	if err != nil {
		return errors.Errorf("fail to get pod (%s): %v", m.Name, err)
	}
	log.Infof("upgrading the etcd member %v from %s to %s", m.Name, member.GetVersion(pod, tp), m.Version)
	pod.Spec.Containers[0].Image = member.MakeImage(m.Version, tp)
	member.SetVersion(pod, m.Version, tp)
	_, err = c.KubeCli.Pods(c.Namespace).Update(pod)
	if err != nil {
		return errors.Errorf("fail to update the etcd member (%s): %v", m.Name, err)
	}
	log.Infof("finished upgrading the etcd member %v", m.Name)
	return nil
}
