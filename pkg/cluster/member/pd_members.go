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
	if len(m.clientURLs) != 0 {
		return strings.Join(m.clientURLs, ",")
	}

	return fmt.Sprintf("http://%s:2379", m.name)
}

func (m *pdMember) peerAddr() string {
	if len(m.PeerURLs) != 0 {
		return strings.Join(m.peerURLs, ",")
	}

	return fmt.Sprintf("http://%s:2380", m.name)
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

func SeedPDMemberset(kubeCli *unversioned.Client, clusterName, nameSpace string, s *spec.ClusterSpec) (MemberSet, error) {
	pms := &PDMemberSet{
		clusterName: clusterName,
		nameSpace:   nameSpace,
		ms:          make(map[string]*pdMember),
		spec:        s,
	}

	err := makeSeedPD()
	return pms, nil
}

func (pms *PDMemberSet) makeSeedPD() error {
	newMemberName := fmt.Sprintf("%s-pd-%04d", pms.clusterName, pms.idCounter)
	pms.idCounter++
	initialCluster := []string{newMemberName + "=http://$(MY_POD_IP):2380"}

	pod := pms.makeSelfHostedEtcdPod(newMemberName, initialCluster, pms.clusterName, "new", uuid.New(), pms.spec)
	_, err := k8sutil.CreateAndWaitPod(pms.kubeCli, pms.namespace, pod, 30*time.Second)
	if err != nil {
		return err
	}

	c.logger.Infof("self-hosted cluster created with seed member (%s)", newMemberName)
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
	for _, m := range pms.MS {
		ps = append(ps, fmt.Sprintf("%s=%s", m.name, m.peerAddr()))
	}
	return ps
}

func (pms *PDMemberSet) ClientURLs() []string {
	endpoints := make([]string, 0, len(pms.MS))
	for _, m := range pms.MS {
		endpoints = append(endpoints, m.clientAddr())
	}
	return endpoints
}

func (pms *PDMemberSet) CreatePodAndService(memberName string, nameSpace string, s *spec.ClusterSpec) error {

	if _, err := k8sutil.CreatePDMemberService(pms.kubeCli, memberName, pms.Name, namespace); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	token := ""
	if state == "new" {
		token = uuid.New()
	}
	pod := k8sutil.MakePDPod(m, pms.PeerURLPairs(), memberName, state, token, s)
	_, err := pms.KubeCli.Pods(namespace).Create(pod)
	return err
}

func (pms *PDMemberSet) SetSpec(s *spec.ClusterSpec) {
	pms.Spec = s
}

func (pms *PDMemberSet) addMember(m *pdMember) {
	pms.MS[m.name] = m
}

func (pms *PDMemberSet) Run(stopC <-chan struct{}, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case <-stopC:
			return
		case <-time.After(5 * time.Second):
			//TODO
			running, pending, err := k8sutil.pollPods()
			if err != nil {
				log.Errorf("fail to poll pods: %v", err)
				continue
			}
			if len(pending) > 0 {
				log.Info("skip reconciliation: running (%v), pending (%v)", k8sutil.GetPodNames(running), k8sutil.GetPodNames(pending))
				continue
			}
			//TODO  alert
			if running == 0 {

			}

			if err := pms.reconcile(running); err != nil {
				log.Errorf("fail to reconcile: %v", err)
				if isFatalError(err) {
					log.Errorf("exiting for fatal error: %v", err)
					return
				}
			}
		}
	}
}

func (pms *PDMemberSet) Size() {
	return len(pms.MS)
}

func (pms *PDMemberSet) makeSelfHostedEtcdPod(name string, initialCluster []string, state, token string) *api.Pod {
	commands := fmt.Sprintf("/bin/pd --data-dir=/var/tidb-cluster/pd --name=%s --initial-advertise-peer-urls=http://$(MY_POD_IP):2380 "+
		"--listen-peer-urls=http://$(MY_POD_IP):2380 --listen-client-urls=http://$(MY_POD_IP):2379 --advertise-client-urls=http://$(MY_POD_IP):2379 "+
		"--initial-cluster=%s --initial-cluster-state=%s",
		name, strings.Join(initialCluster, ","), state)

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
			RestartPolicy: api.RestartPolicyAlways,
			SecurityContext: &api.PodSecurityContext{
				HostNetwork: true,
			},
			Volumes: []api.Volume{
				{Name: "pd-data", VolumeSource: api.VolumeSource{EmptyDir: &api.EmptyDirVolumeSource{}}},
			},
		},
	}

	SetVersion(pod, pms.spec.Pd.Version, pd)

	pod = PodWithAntiAffinity(pod, clusterName)

	if len(cs.NodeSelector) != 0 {
		pod = PodWithNodeSelector(pod, cs.NodeSelector)
	}

	return pod
}
