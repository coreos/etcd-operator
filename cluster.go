package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/etcdserverpb"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/intstr"
)

type clusterEventType string

const (
	eventNewCluster    clusterEventType = "Add"
	eventDeleteCluster clusterEventType = "Delete"
	eventMemberDeleted clusterEventType = "MemberDeleted"
)

type clusterEvent struct {
	typ  clusterEventType
	size int
}

type Cluster struct {
	kclient   *unversioned.Client
	name      string
	idCounter int
	eventCh   chan *clusterEvent
	stopCh    chan struct{}
}

func newCluster(kclient *unversioned.Client, name string, size int) *Cluster {
	c := &Cluster{
		kclient: kclient,
		name:    name,
		eventCh: make(chan *clusterEvent, 100),
		stopCh:  make(chan struct{}),
	}
	go c.run()
	c.send(&clusterEvent{
		typ:  eventNewCluster,
		size: size,
	})
	return c
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

func (c *Cluster) run() {
	go c.monitorMembers()

	for {
		select {
		case event := <-c.eventCh:
			switch event.typ {
			case eventNewCluster:
				c.create(event.size)
			case eventMemberDeleted:

			case eventDeleteCluster:
				c.delete()
				close(c.stopCh)
				return
			}
		}
	}
}

func (c *Cluster) create(size int) {
	initialCluster := []string{}
	for i := 0; i < size; i++ {
		etcdName := fmt.Sprintf("%s-%04d", c.name, i)
		initialCluster = append(initialCluster, fmt.Sprintf("%s=%s", etcdName, makeEtcdPeerAddr(etcdName)))
	}

	for i := 0; i < size; i++ {
		if err := c.launchMember(i, initialCluster, "new"); err != nil {
			// TODO: we need to clean up already created ones.
			panic(err)
		}
	}
	c.idCounter = size
}

func (c *Cluster) launchMember(id int, initialCluster []string, state string) error {
	etcdName := fmt.Sprintf("%s-%04d", c.name, id)
	svc := makeEtcdService(etcdName, c.name)
	if _, err := c.kclient.Services("default").Create(svc); err != nil {
		return err
	}
	pod := makeEtcdPod(etcdName, c.name, initialCluster, state)
	if _, err := c.kclient.Pods("default").Create(pod); err != nil {
		return err
	}
	return nil
}

func (c *Cluster) delete() {
	option := api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": c.name,
		}),
	}

	pods, err := c.kclient.Pods("default").List(option)
	if err != nil {
		panic(err)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		err = c.kclient.Pods("default").Delete(pod.Name, nil)
		if err != nil {
			panic(err)
		}
	}

	services, err := c.kclient.Services("default").List(option)
	if err != nil {
		panic(err)
	}
	for i := range services.Items {
		service := &services.Items[i]
		err = c.kclient.Services("default").Delete(service.Name)
		if err != nil {
			panic(err)
		}
	}
}

func (c *Cluster) monitorMembers() {
	opts := api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": c.name,
		}),
	}
	var prevPods []*api.Pod
	var currPods []*api.Pod
	// TODO: Select "etcd_node" to remove left service.
	for {
		select {
		case <-c.stopCh:
			return
		case <-time.After(3 * time.Second):
		}

		podList, err := c.kclient.Pods("default").List(opts)
		if err != nil {
			panic(err)
		}
		currPods = nil
		for i := range podList.Items {
			currPods = append(currPods, &podList.Items[i])
		}

		// We are recovering one member at a time now.
		deletedPod, remainingPods := findDeletedOne(prevPods, currPods)
		if deletedPod == nil {
			// This will change prevPods if it keeps adding initially.
			prevPods = currPods
			continue
		}
		// currPods could be less than remainingPods.
		prevPods = remainingPods
		// Only using currPods is safe
		if len(currPods) == 0 {
			panic("TODO: All removed. Impossible. Anyway, we can't use etcd client to change membership.")
		}

		// TODO: put this into central event handling
		cfg := clientv3.Config{
			Endpoints: []string{makeClientAddr(currPods[0].Name)},
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			panic(err)
		}
		resp, err := etcdcli.MemberList(context.TODO())
		if err != nil {
			panic(err)
		}

		member := findLostMember(resp.Members, deletedPod.Name)
		_, err = etcdcli.MemberRemove(context.TODO(), member.ID)
		if err != nil {
			panic(err)
		}
		log.Printf("removed member %v with ID %d\n", member.Name, member.ID)

		etcdName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
		initialCluster := buildInitialCluster(resp.Members, member, etcdName)
		_, err = etcdcli.MemberAdd(context.TODO(), []string{makeEtcdPeerAddr(etcdName)})
		if err != nil {
			panic(err)
		}

		log.Printf("added member, cluster: %s", initialCluster)
		c.launchMember(c.idCounter, initialCluster, "existing")
		c.idCounter++
	}
}

func buildInitialCluster(members []*etcdserverpb.Member, removed *etcdserverpb.Member, newMember string) (res []string) {
	for _, m := range members {
		if m.Name == removed.Name {
			continue
		}
		res = append(res, fmt.Sprintf("%s=%s", m.Name, makeEtcdPeerAddr(m.Name)))
	}
	res = append(res, fmt.Sprintf("%s=%s", newMember, makeEtcdPeerAddr(newMember)))
	return res
}

func findLostMember(members []*etcdserverpb.Member, lostMemberName string) *etcdserverpb.Member {
	for _, m := range members {
		if m.Name == lostMemberName {
			return m
		}
	}
	return nil
}

// Find one deleted pod in l2 from l1. Return the deleted pod and the remaining pods.
func findDeletedOne(l1, l2 []*api.Pod) (*api.Pod, []*api.Pod) {
	exist := map[string]struct{}{}
	for _, pod := range l2 {
		exist[pod.Name] = struct{}{}
	}
	for i, pod := range l1 {
		if _, ok := exist[pod.Name]; !ok {
			return pod, append(l1[:i], l1[i+1:]...)
		}
	}
	return nil, l2
}

func makeClientAddr(name string) string {
	return fmt.Sprintf("http://%s:2379", name)
}

func makeEtcdPeerAddr(etcdName string) string {
	return fmt.Sprintf("http://%s:2380", etcdName)
}

func makeEtcdService(etcdName, clusterName string) *api.Service {
	labels := map[string]string{
		"etcd_node":    etcdName,
		"etcd_cluster": clusterName,
	}
	svc := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:   etcdName,
			Labels: labels,
		},
		Spec: api.ServiceSpec{
			Ports: []api.ServicePort{
				{
					Name:       "server",
					Port:       2380,
					TargetPort: intstr.FromInt(2380),
					Protocol:   api.ProtocolTCP,
				},
				{
					Name:       "client",
					Port:       2379,
					TargetPort: intstr.FromInt(2379),
					Protocol:   api.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
	return svc
}

func makeEtcdPod(etcdName, clusterName string, initialCluster []string, state string) *api.Pod {
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: etcdName,
			Labels: map[string]string{
				"app":          "etcd",
				"etcd_node":    etcdName,
				"etcd_cluster": clusterName,
			},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Command: []string{
						"/usr/local/bin/etcd",
						"--name",
						etcdName,
						"--initial-advertise-peer-urls",
						makeEtcdPeerAddr(etcdName),
						"--listen-peer-urls",
						"http://0.0.0.0:2380",
						"--listen-client-urls",
						"http://0.0.0.0:2379",
						"--advertise-client-urls",
						makeClientAddr(etcdName),
						"--initial-cluster",
						strings.Join(initialCluster, ","),
						"--initial-cluster-state",
						state,
					},
					Name:  etcdName,
					Image: "gcr.io/coreos-k8s-scale-testing/etcd-amd64:3.0.4",
					Ports: []api.ContainerPort{
						{
							Name:          "server",
							ContainerPort: int32(2380),
							Protocol:      api.ProtocolTCP,
						},
					},
				},
			},
			RestartPolicy: api.RestartPolicyNever,
		},
	}
	return pod
}
