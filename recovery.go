package main

import (
	"fmt"
	"log"

	"golang.org/x/net/context"
)

// reconcile reconciles the members in the cluster view with running pods in
// Kubernetes.
//
// Definitions:
// - running pods in k8s cluster
// - members in controller knowledge
// Steps:
// 1. Remove all pods from set running that does not belong to set members
// 2. R’ consist of remaining pods of runnings
// 3. If R’ = members the current state matches the membership state. END.
// 4. If len(R’) < len(members)/2 + 1, quorum lost. Go to recovery process (TODO).
// 5. Add one missing member. END.
func (c *Cluster) reconcile() error {
	defer func() {
		log.Println("Finish Reconciling\n")
	}()

	fmt.Printf("cluster size: current = %d , desired = %d\n", len(c.members), c.size)

	//If any resize operations take place, we want to bail after and let member reconcilation occur next time around
	if len(c.members) < c.size {
		//scale up
		for i := len(c.members); i < c.size; i++ {
			if err := c.incrementNodeCount(); err != nil {
				return err
			}
		}
		return nil
	} else if len(c.members) > c.size {
		//scale down
		for i := len(c.members); i > c.size; i-- {
			if err := c.decrementNodeCount(); err != nil {
				return err
			}
		}
		return nil
	}

	running := c.getRunningEtcdPods()

	log.Println("Reconciling cluster state:")
	log.Println("\tRunning pods:", running)
	log.Println("\tExpected membership:", c.members)

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
		fmt.Println("Match")
		return nil
	}

	if L.Size() < c.members.Size()/2+1 {
		fmt.Println("Disaster recovery")
		return c.disasterRecovery()
	}

	fmt.Println("Recovery one member")
	toRecover := c.members.Diff(L).PickOne()
	return c.recoverOneMember(toRecover)
}

func (c *Cluster) recoverOneMember(toRecover *Member) error {
	clustercli, err := getEtcdClusterClient(c.members.ClientURLs(), 15)
	if err != nil {
		return err
	}
	// Remove toRecover membership first since it's gone
	if _, err := clustercli.MemberRemove(context.TODO(), toRecover.ID); err != nil {
		return err
	}
	c.members.Remove(toRecover.Name)
	log.Printf("removed member (%v) with ID (%d)\n", toRecover.Name, toRecover.ID)

	// Add a new member
	newMemberName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	newMember := &Member{Name: newMemberName}
	resp, err := clustercli.MemberAdd(context.TODO(), []string{newMember.PeerAddr()})
	if err != nil {
		panic(err)
	}
	newMember.ID = resp.Member.ID
	c.members.Add(newMember)

	if err := c.createPodAndService(newMember, "existing"); err != nil {
		return err
	}
	c.idCounter++

	log.Printf("added member, cluster: %s", c.members.PeerURLPairs())
	return nil
}

func (c *Cluster) disasterRecovery() error {
	panic("unimplemented disaster recovery")
}
