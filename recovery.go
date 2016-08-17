package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/coreos/etcd/clientv3"
	"golang.org/x/net/context"
)

type Member struct {
	Name string
	// Note: ID might not exist
	ID uint64
}

type MemberSet []Member

// the set of all members of s1 that are not members of s2
func (s1 MemberSet) Diff(s2 MemberSet) MemberSet {
	res := MemberSet{}
	for _, m1 := range s1 {
		if !s2.Has(m1) {
			res = append(res, m1)
		}
	}
	return res
}

func (ms MemberSet) Size() int {
	return len(ms)
}

func (ms MemberSet) Has(check Member) bool {
	for _, m := range ms {
		// We use Name instead of ID because ID might not exist in Member struct
		if check.Name == m.Name {
			return true
		}
	}
	return false
}

func (ms MemberSet) String() string {
	var mstring []string

	for _, m := range ms {
		mstring = append(mstring, m.Name)
	}
	return strings.Join(mstring, ",")
}

func PickOne(ms MemberSet) Member {
	return ms[0]
}

// Definitions:
// - running pods P in k8s cluster
// - membership M in controller knowledge
// Steps:
// 1. Remove all pods from set P that does not belong to set M
// 2. P’ consist of remaining pods of P
// 3. If P’ = M, the current state matches the membership state. END.
// 4. If len(P’) < len(M)/2 + 1, quorum lost. Go to recovery process (TODO).
// 5. Add one missing member. END.
func (c *Cluster) reconcile(P, M MemberSet) error {
	log.Println("Reconciling:")
	log.Println("Running pods:", P)
	log.Println("Expected membership:", M)

	defer func() {
		log.Println("Finish Reconciling\n")
	}()

	unknownMembers := P.Diff(M)
	if unknownMembers.Size() > 0 {
		log.Println("Removing unexpected pods:", unknownMembers)
		if err := c.removePods(unknownMembers); err != nil {
			return err
		}
	}
	L := P.Diff(unknownMembers)
	if L.Size() == M.Size() {
		fmt.Println("Match")
		return nil
	}

	if L.Size() < M.Size()/2+1 {
		fmt.Println("Disaster recovery")
		return c.disasterRecovery()
	}

	fmt.Println("Recovery one member")
	toRecover := PickOne(M.Diff(L))
	return c.recoverOneMember(toRecover, M)
}

func (c *Cluster) recoverOneMember(toRecover Member, M MemberSet) error {
	// Remove toRecover membership first since it's gone
	cfg := clientv3.Config{
		Endpoints: []string{makeClientAddr(M[0].Name)},
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}

	_, err = etcdcli.MemberRemove(context.TODO(), toRecover.ID)
	if err != nil {
		return err
	}
	log.Printf("removed member (%v) with ID (%d)\n", toRecover.Name, toRecover.ID)

	// Add a new member
	newMember := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	_, err = etcdcli.MemberAdd(context.TODO(), []string{makeEtcdPeerAddr(newMember)})
	if err != nil {
		panic(err)
	}
	initialCluster := buildInitialCluster(M, toRecover.Name, newMember)
	if err := c.launchNewMember(c.idCounter, initialCluster, "existing"); err != nil {
		return err
	}
	c.idCounter++
	log.Printf("added member, cluster: %s", initialCluster)
	return nil
}

func buildInitialCluster(ms MemberSet, removed, newMember string) (res []string) {
	for _, m := range ms {
		if m.Name == removed {
			continue
		}
		res = append(res, fmt.Sprintf("%s=%s", m.Name, makeEtcdPeerAddr(m.Name)))
	}
	res = append(res, fmt.Sprintf("%s=%s", newMember, makeEtcdPeerAddr(newMember)))
	return res
}

func (c *Cluster) removePods(ms MemberSet) error {
	for _, m := range ms {
		err := c.kclient.Pods("default").Delete(m.Name, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Cluster) disasterRecovery() error {
	panic("unimplemented disaster recovery")
}
