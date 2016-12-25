package member

import (
	"fmt"

	"github.com/GregoryIan/oprator/spec"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type pdMember struct {
	name       string
	id         uint64
	perrURLs   []string
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
	nameSpace   string
	idCounter   int
	ms          map[string]*pdMember
	spec        *spec.ClusterSpec
	kubeCli     *unversioned.Client
}

func init() {
	RegisterSeedMemberFunc(PD, SeedPDMemberset)
}

func NewPDMemberset(kubeCli *unversioned.Client, clusterName, nameSpace string, s *spec.ClusterSpec) (MemberSet, error) {
	pms := &PDMemberSet{
		clusterName: clusterName,
		nameSpace:   nameSpace,
		ms:          make(map[string]*pdMember),
		spec:        s,
	}

	err := newSeedMember()
	if err != nil {
		return err
	}

	return pms, nil
}

func (pms *PDMemberSet) newSeedMember() (*pdMember, error) {
	newMemberName := fmt.Sprintf("%s-pd-%04d", pms.clusterName, pms.idCounter)
	pms.idCounter++
	initialCluster := []string{newMemberName + "=http://$(MY_POD_IP):2380"}

	pod := pms.makePDMember(newMemberName, initialCluster, pms.clusterName, "new", uuid.New(), pms.spec)
	_, err := CreateAndWaitPod(pms.kubeCli, pms.namespace, pod, 30*time.Second)
	if err != nil {
		return nil, err
	}

	log.Infof("tidb cluster created with seed pd member (%s)", newMemberName)
	return nil
}

func (pms *PDMemberSet) AddOneMember() error {
	newMemberName := fmt.Sprintf("%s-pd-%04d", pms.clusterName, pms.idCounter)
	pms.idCounter++

	peerURL := "http://$(MY_POD_IP):2380"
	initialCluster := append(pms.PeerURLPairs(), newMemberName+"="+peerURL)

	pod := pms.makePDMember(newMemberName, initialCluster, c.Name, "existing", "", pms.spec)
	pod = PodWithAddMemberInitContainer(pod, pms.ClientURLs(), newMemberName, []string{peerURL}, c.spec)

	_, err := pms.KubeCli.Pods(pms.namespace).Create(pod)
	if err != nil {
		return err
	}
	// wait for the new pod to start and add itself into the etcd cluster.
	cfg := clientv3.Config{
		Endpoints:   c.members.ClientURLs(),
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	oldN := c.members.Size()
	// TODO: do not wait forever.
	for {
		err = pms.updateMembers(etcdcli)
		// TODO: check error type to determine the etcd member is not ready.
		if err != nil && c.members != nil {
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

func (pms *PDMemberSet) UpdateMembers(etcdcli *clientv3.Client) error {
	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := cli.MemberList(ctx)
	if err != nil {
		return err
	}
	pms = &pdMemberSet{}
	for _, m := range resp.Members {
		if len(m.Name) == 0 {
			pms.MS = nil
			return fmt.Errorf("the name of member (%x) is empty. Not ready yet. Will retry later", m.ID)
		}
		id := findID(m.Name)
		if id+1 > pms.idCounter {
			pms.idCounter = id + 1
		}

		pms.ms[m.Name] = &pdMember{
			Name:       m.Name,
			ID:         m.ID,
			ClientURLs: m.ClientURLs,
			PeerURLs:   m.PeerURLs,
		}
	}
	return nil
}

func (pms *PDMemberSet) Diff(other MemberSet) {
	o, ok := other.(*PDMemberSet)
	if !ok {
		log.Errorf("other(%v) is not a PDMemberSet", other)
	}
	s := &PDMemberSet{
		MS: make(map[string]*pdMember),
	}

	for n, m := range pms.MS {
		if _, ok := o.MS[n]; !ok {
			s.MS[n] = m
		}
	}
	return s
}

func (pms *PDMemberSet) PeerURLPairs() []string {
	ps := make([]string, 0)
	for _, m := range pms.ms {
		ps = append(ps, fmt.Sprintf("%s=%s", m.name, m.peerAddr()))
	}
	return ps
}

func (pms *PDMemberSet) ClientURLs() []string {
	endpoints := make([]string, 0, len(pms.MS))
	for _, m := range pms.ms {
		endpoints = append(endpoints, m.clientAddr())
	}
	return endpoints
}

func (pms *PDMemberSet) Remove(name string) {
	delete(pms.ms, name)
}

func (pms *PDMemberSet) PickOne() *pdMember {
	for _, m := range pms.ms {
		return m
	}
	panic("empty")
}

func (pms *PDMemberSet) SetSpec(s *spec.ClusterSpec) {
	pms.Spec = s
}

func (pms *PDMemberSet) Size() {
	return len(pms.MS)
}

func (pms *PDMemberSet) makeMember(name string, initialCluster []string, state, token string) *api.Pod {
	commands := fmt.Sprintf("/bin/pd --data-dir=%s/pd --name=%s --initial-advertise-peer-urls=http://$(MY_POD_IP):2380 "+
		"--listen-peer-urls=http://$(MY_POD_IP):2380 --listen-client-urls=http://$(MY_POD_IP):2379 --advertise-client-urls=http://$(MY_POD_IP):2379 "+
		"--initial-cluster=%s --initial-cluster-state=%s",
		tidbClusterDir, name, strings.Join(initialCluster, ","), state)

	if state == "new" {
		commands = fmt.Sprintf("%s --initial-cluster-token=%s", commands, token)
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
					Image:   MakeImage(pms.Pd.Version, PD),
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
			RestartPolicy: api.RestartPolicyNever,
			Volumes: []api.Volume{
				{Name: "pd-data", VolumeSource: api.VolumeSource{EmptyDir: &api.EmptyDirVolumeSource{}}},
			},
		},
	}

	SetVersion(pod, pms.spec.Pd.Version, pd)
	pod = PodWithAntiAffinity(pod, pms.clusterName)

	if len(pms.spec.NodeSelector) != 0 {
		pod = PodWithNodeSelector(pod, pms.spec.NodeSelector)
	}

	return pod
}
