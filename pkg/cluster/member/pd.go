package member

import (
	"fmt"
	"strings"
	"time"

	"github.com/GregoryIan/operator/pkg/spec"
	"github.com/juju/errors"
	"github.com/ngaut/log"
	"github.com/pingcap/pd/pd-client"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type pdMember struct {
	name       string
	id         uint64
	peerURLs   []string
	clientURLs []string
}

func (m *pdMember) clientAddr() string {
	return strings.Join(m.clientURLs, ",")
}

func (m *pdMember) peerAddr() string {
	return strings.Join(m.peerURLs, ",")
}

type PDMemberSet struct {
	clusterName string
	namespace   string
	idCounter   int
	members     map[string]*pdMember
	spec        *spec.ServiceSpec
	kubeCli     *unversioned.Client
}

func init() {
	RegisterSeedMemberFunc(PD, newPDMemberset)
}

func newPDMemberset(kubeCli *unversioned.Client, clusterName, namespace string, s *spec.ClusterSpec) (MemberSet, error) {
	pms := &PDMemberSet{
		clusterName: clusterName,
		namespace:   namespace,
		members:     make(map[string]*pdMember),
		spec:        s.PD,
		kubeCli:     kubeCli,
	}
	err := pms.newSeedMember()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return pms, nil
}

func (pms *PDMemberSet) RestoreFromPods(pods []*api.Pod) {
	pms.members = make(map[string]*pdMember)
	for _, pod := range pods {
		m := &pdMember{name: pod.Name}
		m.clientURLs = []string{"http://" + pod.Status.PodIP + ":2379"}
		m.peerURLs = []string{"http://" + pod.Status.PodIP + ":2380"}
		pms.members[pod.Name] = m
	}
}

func (pms *PDMemberSet) newSeedMember() error {
	newMemberName := fmt.Sprintf("%s-pd-%04d", pms.clusterName, pms.idCounter)
	pms.idCounter++
	initialCluster := []string{newMemberName + "=http://$(MY_POD_IP):2380"}

	pod := pms.makePDMember(newMemberName, initialCluster)
	_, err := CreateAndWaitPod(pms.kubeCli, pms.namespace, pod, 30*time.Second)
	if err != nil {
		return errors.Trace(err)
	}

	log.Infof("tidb cluster created with seed pd member (%s)", newMemberName)
	return nil
}

func (pms *PDMemberSet) NewOneMember() error {
	newMemberName := fmt.Sprintf("%s-pd-%04d", pms.clusterName, pms.idCounter)
	pms.idCounter++
	peerURL := "http://$(MY_POD_IP):2380"
	initialCluster := append(pms.PeerURLPairs(), newMemberName+"="+peerURL)

	pod := pms.makePDMember(newMemberName, initialCluster)
	pod = PodWithAddMemberInitContainer(pod, pms.ClientURLs(), newMemberName, []string{peerURL}, pms.spec)

	_, err := pms.kubeCli.Pods(pms.namespace).Create(pod)
	if err != nil {
		return errors.Trace(err)
	}
	log.Fatal("finish create pod!")
	// wait for the new pod to start and add itself into the etcd cluster.
	cli, err := pd.NewClient(pms.ClientURLs())
	if err != nil {
		return errors.Trace(err)
	}
	defer cli.Close()

	oldN := pms.Size()
	// TODO: do not wait forever.
	for {
		err = pms.UpdateMembers(cli)
		// TODO: check error type to determine the etcd member is not ready.
		if err != nil && pms.members != nil {
			log.Errorf("failed to list pd members for update: %v", err)
			return err
		}
		if pms.Size() > oldN {
			break
		}
		time.Sleep(5 * time.Second)
	}

	log.Infof("added a pd member (%s)", newMemberName)
	return nil
}

func (pms *PDMemberSet) UpdateMembers(cli pd.Client) error {
	/*ctx, _ := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	resp, err := cli.MemberList(ctx)
	if err != nil {
		return err
	}
	pms.members = make(map[string]*pdMember)
	for _, m := range resp.Members {
		if len(m.Name) == 0 {
			pms.members = nil
			return errors.Errorf("the name of member (%x) is empty. Not ready yet. Will retry later", m.ID)
		}
		id := findID(m.Name)
		if id+1 > pms.idCounter {
			pms.idCounter = id + 1
		}

		pms.members[m.Name] = &pdMember{
			name:       m.Name,
			id:         m.ID,
			clientURLs: m.ClientURLs,
			peerURLs:   m.PeerURLs,
		}
	}*/
	return nil
}

func (pms *PDMemberSet) Diff(other MemberSet) MemberSet {
	o, ok := other.(*PDMemberSet)
	if !ok {
		log.Errorf("other(%v) is not a PDMemberSet", other)
	}
	s := &PDMemberSet{
		members: make(map[string]*pdMember),
	}

	for n, m := range pms.members {
		if _, ok := o.members[n]; !ok {
			s.members[n] = m
		}
	}
	return s
}

func (pms *PDMemberSet) PeerURLPairs() []string {
	ps := make([]string, 0)
	for _, m := range pms.members {
		ps = append(ps, fmt.Sprintf("%s=%s", m.name, m.peerAddr()))
	}
	return ps
}

func (pms *PDMemberSet) ClientURLs() []string {
	endpoints := make([]string, 0, len(pms.members))
	for _, m := range pms.members {
		endpoints = append(endpoints, m.clientAddr())
	}
	return endpoints
}

func (pms *PDMemberSet) RemoveMemberShip(name string) {
	delete(pms.members, name)
}

func (pms *PDMemberSet) PickOne() *Member {
	for _, m := range pms.members {
		return &Member{
			Name: m.name,
			ID:   m.id,
		}
	}
	panic("empty")
}

func (pms *PDMemberSet) SetSpec(s *spec.ServiceSpec) {
	pms.spec = s
}

func (pms *PDMemberSet) Size() int {
	return len(pms.members)
}

func (pms *PDMemberSet) NeedUpgrade(pods []*api.Pod) bool {
	return !pms.NotEqualPodSize(len(pods)) && pms.PickOneOldMember(pods) != nil
}

func (pms *PDMemberSet) NotEqualPodSize(size int) bool {
	return size != pms.spec.Size
}

func (pms *PDMemberSet) PickOneOldMember(pods []*api.Pod) *Member {
	for _, pod := range pods {
		if GetVersion(pod, PD) == pms.spec.Version {
			continue
		}
		return &Member{Name: pod.Name, Version: pms.spec.Version}
	}
	return nil
}

func (pms *PDMemberSet) Members() []*Member {
	var members = make([]*Member, len(pms.members))
	index := 0
	for _, m := range pms.members {
		members[index] = &Member{Name: m.name}
		index++
	}

	return members
}

func (pms *PDMemberSet) makePDMember(name string, initialCluster []string) *api.Pod {
	commands := fmt.Sprintf("/pd-server --data-dir=%s/pd --name=%s --advertise-peer-urls=http://$(MY_POD_IP):2380 "+
		"--peer-urls=http://$(MY_POD_IP):2380 --client-urls=http://$(MY_POD_IP):2379 --advertise-client-urls=http://$(MY_POD_IP):2379 "+
		"--initial-cluster=%s",
		tidbClusterDir, name, strings.Join(initialCluster, ","))

	version := defaultVersion
	if pms.spec != nil && len(pms.spec.Version) > 0 {
		version = pms.spec.Version
	}

	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app":          "pd",
				"pd-node":      name,
				"tidb_cluster": pms.clusterName,
			},
			Annotations: map[string]string{},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					// TODO: fix "sleep 5".
					// Without waiting some time, there is highly probable flakes in network setup.
					Command: []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s", commands)},
					Name:    name,
					Image:   MakeImage(version, PD),
					Ports: []api.ContainerPort{
						{
							Name:          "server",
							ContainerPort: int32(2380),
							Protocol:      api.ProtocolTCP,
						},
						{
							Name:          "client",
							ContainerPort: int32(2379),
							Protocol:      api.ProtocolTCP,
						},
					},
					VolumeMounts: []api.VolumeMount{
						{Name: "pd-data", MountPath: tidbClusterDir},
					},
					Env: []api.EnvVar{envPodIP},
				},
			},
			RestartPolicy: api.RestartPolicyAlways,
			Volumes: []api.Volume{
				{Name: "pd-data", VolumeSource: api.VolumeSource{EmptyDir: &api.EmptyDirVolumeSource{}}},
			},
		},
	}

	SetVersion(pod, version, PD)
	pod = PodWithAntiAffinity(pod, pms.clusterName)

	if len(pms.spec.NodeSelector) != 0 {
		pod = PodWithNodeSelector(pod, pms.spec.NodeSelector)
	}

	return pod
}
