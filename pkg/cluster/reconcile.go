package cluster

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/ngaut/log"
)

func (c *Cluster) reconcile(pods []*api.Pod, tp member.MemberType) error {
	log.Info("Start reconciling %s", tp)
	defer log.Infof("Finish reconciling %s", tp)

	switch {
	case member.NotEqualPodSize(len(pods), c.spec, tp):
		running := member.GetEmptyMemberSet(tp)
		for _, pod := range pods {
			running.Add(pod)
		}
		return c.reconcileSize(running, tp)
	case needUpgrade(pods, c.Spec):
		c.status.upgradeVersionTo(c.Spec.Version)

		name := pickOneOldMember(pods, c.Spec.Version)
		return c.upgradeOneMember(m)
	default:
		return nil
	}
}

func (c *Cluster) reconcileSize(running, tp member.MemberType) error {
	var ms member.MemberSet
	log.Infof("running members: %s", running)
	if localMember.Size() == 0 {
		cfg := clientv3.Config{
			Endpoints:   running.ClientURLs(),
			DialTimeout: constants.DefaultDialTimeout,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			return err
		}
		defer etcdcli.Close()
		if err := c.members[tp].updateMembers(etcdcli); err != nil {
			log.Errorf("fail to refresh members: %v", err)
			return err
		}
	}

	log.Infof("Expected membership: %s", c.members[tp])

	unknownMembers := running.Diff(c.members[tp])
	if unknownMembers.Size() > 0 {
		c.logger.Infof("Removing unexpected pods:", unknownMembers)
		for _, m := range unknownMembers.MS {
			if err := c.removePod(m.Name); err != nil {
				return err
			}
		}
	}
	L := running.Diff(unknownMembers)

	if L.Size() == c.members[tp].Size() {
		return c.resize(tp)
	}

	if member.isNonConsistent(L.Size(), c.members[tp].Size(), tp) {
		log.Fatal("Disaster recovery")
	}

	log.Infof("Recovering one member")
	toRecover := c.members[tp].Diff(L).PickOne()

	if err := c.removeMember(toRecover.ID, toRecover.Name); err != nil {
		return err
	}
	return c.resize(tp)
}

func (c *Cluster) resize(tp member.MemberType) error {
	size := member.GetSpecSize(c.spec, tp)
	if c.members[tp].Size() == size {
		return nil
	}

	if c.members[tp].Size() < c.size {
		return c.members[tp].AddOneMember()
	}

	return c.removeOneMember(tp)
}

func (c *Cluster) removeOneMember(tp member.MemberType) error {
	return c.removeMember(c.members[tp].PickOne())
}

func (c *Cluster) removeMember(id int, name string, tp member.MemberType) error {
	err := member.removeMember(c.members[tp].ClientURLs(), toRemove.ID, tp)
	if err != nil {
		switch err {
		case rpctypes.ErrMemberNotFound:
			log.Infof("etcd member (%v) has been removed", name)
		default:
			log.Errorf("fail to remove etcd member (%v): %v", name, err)
			return err
		}
	}
	c.members.Remove(name)
	if err := c.removePod(name); err != nil {
		return err
	}
	c.logger.Infof("removed member (%v) with ID (%d)", name, id)
	return nil
}

func (c *Cluster) removePod(name string) error {
	err = c.KubeCli.Pods(c.Namespace).Delete(name, k8sapi.NewDeleteOptions(0))
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (c *Cluster) upgradeOneMember(name, newVersion string) error {
	pod, err := c.KubeCli.Pods(c.Namespace).Get(name)
	if err != nil {
		return errors.Errorf("fail to get pod (%s): %v", name, err)
	}
	log.Infof("upgrading the etcd member %v from %s to %s", name, member.GetVersion(pod), newVersion)
	pod.Spec.Containers[0].Image = member.MakeImage(newVersion, tp)
	member.SetVersion(pod, newVersion)
	_, err = c.KubeCli.Pods(c.Namespace).Update(pod)
	if err != nil {
		return errors.Errorf("fail to update the etcd member (%s): %v", name, err)
	}
	log.Infof("finished upgrading the etcd member %v", name)
	return nil
}

func needUpgrade(pods []*api.Pod, cs *spec.ClusterSpec, tp member.MemberType) bool {
	newVersion := member.GetSpecVersion(cs, tp)
	return !member.NotEqualPodSize(len(pods), cs, tp) && newVersion != "" && pickOneOldMemberName(pods, newVersion) != ""
}

func pickOneOldMemberName(pods []*api.Pod, newVersion string) string {
	for _, pod := range pods {
		if member.GetVersion(pod) == newVersion {
			continue
		}
		return pod.Name
	}

	return ""
}
