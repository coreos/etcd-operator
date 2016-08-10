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

type Cluster struct {
	kclient   *unversioned.Client
	name      string
	idCounter int
	eventCh   chan *Event
	stopCh    chan struct{}
}

func newCluster(kclient *unversioned.Client, name string) *Cluster {
	return &Cluster{
		kclient: kclient,
		name:    name,
		eventCh: make(chan *Event, 100),
		stopCh:  make(chan struct{}),
	}
}

func (c *Cluster) Run() {
	go c.monitorMembers()

	for {
		select {
		case event := <-c.eventCh:
			switch event.Type {
			case "ADDED":
				c.createCluster(event.Object)
			case "DELETED":
				c.deleteCluster(event.Object)
			}
		case <-c.stopCh:
		}
	}
}

func (c *Cluster) Stop() {
	close(c.stopCh)
}

func (c *Cluster) Handle(ev *Event) {
	select {
	case c.eventCh <- ev:
	case <-c.stopCh:
	default:
		panic("TODO: too many events queued...")
	}
}

func (c *Cluster) createCluster(cluster EtcdCluster) {
	size := cluster.Size
	clusterName := cluster.Metadata["name"]

	initialCluster := []string{}
	for i := 0; i < size; i++ {
		initialCluster = append(initialCluster, fmt.Sprintf("%s-%04d=http://%s-%04d:2380", clusterName, i, clusterName, i))
	}

	for i := 0; i < size; i++ {
		c.launchMember(clusterName, i, initialCluster, "new")
	}
	c.idCounter = size
}

func (c *Cluster) launchMember(clusterName string, id int, initialCluster []string, state string) {
	etcdName := fmt.Sprintf("%s-%04d", clusterName, id)
	svc := makeEtcdService(etcdName, clusterName)
	_, err := c.kclient.Services("default").Create(svc)
	if err != nil {
		panic(err)
	}
	pod := makeEtcdPod(etcdName, clusterName, initialCluster, state)
	_, err = c.kclient.Pods("default").Create(pod)
	if err != nil {
		panic(err)
	}
}

func (c *Cluster) deleteCluster(cluster EtcdCluster) {
	clusterName := cluster.Metadata["name"]
	option := api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": clusterName,
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
	// TODO: assuming delete one pod and add one pod. Handle more complex case later.
	// TODO: What about unremoved service?
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

		// DEBUGGING..
		fmt.Printf("previous pods: ")
		for _, pod := range prevPods {
			fmt.Printf("%s, ", pod.Name)
		}
		fmt.Println("")

		fmt.Printf("current pods: ")
		for _, pod := range currPods {
			fmt.Printf("%s, ", pod.Name)
		}
		fmt.Println("")

		deletedPod := findDeleted(prevPods, currPods)
		prevPods = currPods
		if deletedPod == nil {
			continue
		}
		if len(currPods) == 0 {
			panic("unexpected")
		}

		cfg := clientv3.Config{
			Endpoints: []string{fmt.Sprintf("http://%s:2379", currPods[0].Name)},
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			panic(err)
		}
		resp, err := etcdcli.MemberList(context.TODO())
		if err != nil {
			panic(err)
		}

		member := findLostMember(resp.Members, deletedPod)
		_, err = etcdcli.MemberRemove(context.TODO(), member.ID)
		if err != nil {
			panic(err)
		}
		log.Printf("removed member %v with ID %d\n", member.Name, member.ID)

		etcdName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
		initialCluster := buildInitialClusters(resp.Members, member, etcdName)
		_, err = etcdcli.MemberAdd(context.TODO(), []string{fmt.Sprintf("http://%s:2380", etcdName)})
		if err != nil {
			panic(err)
		}

		log.Printf("added member, cluster: %s", initialCluster)
		c.launchMember(c.name, c.idCounter, initialCluster, "existing")
		c.idCounter++
	}
}

func buildInitialClusters(members []*etcdserverpb.Member, removed *etcdserverpb.Member, newMember string) (res []string) {
	for _, m := range members {
		if m.Name == removed.Name {
			continue
		}
		res = append(res, fmt.Sprintf("%s=http://%s:2380", m.Name, m.Name))
	}
	res = append(res, fmt.Sprintf("%s=http://%s:2380", newMember, newMember))
	return res
}

func findLostMember(members []*etcdserverpb.Member, deletedPod *api.Pod) *etcdserverpb.Member {
	for _, m := range members {
		if m.Name == deletedPod.Name {
			return m
		}
	}
	return nil
}

func findDeleted(pl1, pl2 []*api.Pod) *api.Pod {
	exist := map[string]struct{}{}
	for _, pod := range pl2 {
		exist[pod.Name] = struct{}{}
	}
	for _, pod := range pl1 {
		if _, ok := exist[pod.Name]; !ok {
			return pod
		}
	}
	return nil
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
						fmt.Sprintf("http://%s:2380", etcdName),
						"--listen-peer-urls",
						"http://0.0.0.0:2380",
						"--listen-client-urls",
						"http://0.0.0.0:2379",
						"--advertise-client-urls",
						fmt.Sprintf("http://%s:2379", etcdName),
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
