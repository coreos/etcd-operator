package main

import (
	"fmt"
	"log"
	"time"

	"github.com/coreos/etcd/clientv3"
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
func (c *Cluster) reconcile(running MemberSet) error {
	log.Println("Reconciling:")
	log.Println("Running pods:", running)
	log.Println("Expected membership:", c.members)

	defer func() {
		log.Println("Finish Reconciling\n")
	}()

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

func (c *Cluster) recoverOneMember(toRecover Member) error {
	// Remove toRecover membership first since it's gone
	cfg := clientv3.Config{
		Endpoints:   c.members.ClientURLs(),
		DialTimeout: 5 * time.Second,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	clustercli := clientv3.NewCluster(etcdcli)
	_, err = clustercli.MemberRemove(context.TODO(), toRecover.ID)
	if err != nil {
		return err
	}
	log.Printf("removed member (%v) with ID (%d)\n", toRecover.Name, toRecover.ID)
	c.members.Remove(toRecover.Name)

	// Add a new member
	newMember := fmt.Sprintf("%s-%04d", c.name, c.idCounter)

	resp, err := clustercli.MemberAdd(context.TODO(), []string{makeEtcdPeerAddr(newMember)})
	if err != nil {
		panic(err)
	}

	c.members.Add(Member{Name: newMember, ID: resp.Member.ID})

	initialCluster := c.members.PeerURLPairs()
	if err := c.createPodAndService(c.idCounter, initialCluster, "existing"); err != nil {
		return err
	}
	c.idCounter++

	log.Printf("added member, cluster: %s", initialCluster)
	return nil
}

func (c *Cluster) disasterRecovery() error {
	panic("unimplemented disaster recovery")
}
