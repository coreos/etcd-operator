package main

import (
	"fmt"
	"log"
	"time"

	"github.com/coreos/etcd/clientv3"
	"golang.org/x/net/context"
)

// reconcile reconciles
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
// 4. If len(L) < len(members)/2 + 1, quorum lost. Go to recovery process (TODO).
// 5. Add one missing member. END.
func (c *Cluster) reconcile(running MemberSet) error {
	log.Println("Reconciling:")
	defer func() {
		log.Println("Finish Reconciling\n")
	}()

	if len(c.members) == 0 {
		cfg := clientv3.Config{
			Endpoints:   running.ClientURLs(),
			DialTimeout: 5 * time.Second,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			return err
		}
		if err := waitMemberReady(etcdcli); err != nil {
			return err
		}
		c.updateMembers(etcdcli)
	}

	log.Println("Running pods:", running)
	log.Println("Expected membership:", c.members)

	unknownMembers := running.Diff(c.members)
	if unknownMembers.Size() > 0 {
		log.Println("Removing unexpected pods:", unknownMembers)
		for _, m := range unknownMembers {
			if err := c.removePodAndService(m.Name); err != nil {
				panic(err)
			}
		}
	}
	L := running.Diff(unknownMembers)

	if L.Size() == c.members.Size() {
		return c.resize()
	}

	if L.Size() < c.members.Size()/2+1 {
		fmt.Println("Disaster recovery")
		return c.disasterRecovery()
	}

	fmt.Println("Recovering one member")
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
		DialTimeout: 5 * time.Second,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	newMemberName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	newMember := &Member{Name: newMemberName}
	resp, err := etcdcli.MemberAdd(context.TODO(), []string{newMember.PeerAddr()})
	if err != nil {
		panic(err)
	}
	newMember.ID = resp.Member.ID
	c.members.Add(newMember)

	if err := c.createPodAndService(c.members, newMember, "existing"); err != nil {
		return err
	}
	c.idCounter++
	log.Printf("added member, cluster: %s", c.members.PeerURLPairs())
	return nil
}

func (c *Cluster) removeOneMember() error {
	return c.removeMember(c.members.PickOne())
}

func (c *Cluster) removeMember(toRemove *Member) error {
	cfg := clientv3.Config{
		Endpoints:   c.members.ClientURLs(),
		DialTimeout: 5 * time.Second,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}

	clustercli := clientv3.NewCluster(etcdcli)
	if _, err := clustercli.MemberRemove(context.TODO(), toRemove.ID); err != nil {
		return err
	}
	c.members.Remove(toRemove.Name)
	if err := c.removePodAndService(toRemove.Name); err != nil {
		return err
	}
	log.Printf("removed member (%v) with ID (%d)\n", toRemove.Name, toRemove.ID)
	return nil
}

func (c *Cluster) disasterRecovery() error {
	panic("unimplemented disaster recovery")
}
