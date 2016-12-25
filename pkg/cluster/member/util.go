package member

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/kubernetes/pkg/util/wait"
)

var (
	tidbClusterDir = "/var/tidb-cluster"
	envPodIP       = api.EnvVar{
		Name: "MY_POD_IP",
		ValueFrom: &api.EnvVarSource{
			FieldRef: &api.ObjectFieldSelector{
				FieldPath: "status.podIP",
			},
		},
	}
)

func isNonConsistent(size, localSize int, tp MemberType) bool {
	switch {
	case PD:
		return size < localSize/2+1
	}
}

func NotEqualPodSize(size int, s *spec.ClusterSpec, tp MemberType) {
	switch {
	case PD:
		return size != s.Pd.Size
	}

	return false
}

func GetSpecVersion(s *spec.ClusterSpec, tp MemberType) string {
	switch {
	case PD:
		return s.Pd.Version
	}

	return ""
}

func GetSpecSize(s *spec.ClusterSpec, tp MemberType) string {
	switch {
	case PD:
		return s.Pd.Size
	}

	return 0
}

func GetVersion(pod *api.Pod, tp ServiceType) string {
	return pod.Annotations[fmt.Sprintf("%s.version", tp)]
}

func SetVersion(pod *api.Pod, version string, tp MemberType) {
	pod.Annotations[fmt.Sprintf("%s.version", tp)] = version
}

func MakeImage(version string, tp MemberType) string {
	return fmt.Sprintf("pingcap/%s:%v", tp, version)
}

func GetPodNames(pods []*api.Pod) []string {
	res := []string{}
	for _, p := range pods {
		res = append(res, p.Name)
	}
	return res
}

func CreateTiDBNodePortService(kclient *unversioned.Client, tidbName, clusterName, ns string) (*api.Service, error) {
	svc := makeTiDBNodePortService(tidbName, clusterName)
	return kclient.Services(ns).Create(svc)
}

func PodWithAddMemberInitContainer(p *api.Pod, endpoints []string, name string, peerURLs []string, cs *spec.ClusterSpec) *api.Pod {
	containerSpec := []api.Container{
		{
			Name:  "add-member",
			Image: MakeEtcdImage(cs.Version),
			Command: []string{
				"/bin/sh", "-c",
				fmt.Sprintf("pdctl --endpoints=%s member add %s --peer-urls=%s", strings.Join(endpoints, ","), name, strings.Join(peerURLs, ",")),
			},
			Env: []api.EnvVar{envPodIP},
		},
	}
	b, err := json.Marshal(containerSpec)
	if err != nil {
		panic(err)
	}
	p.Annotations[k8sv1api.PodInitContainersAnnotationKey] = string(b)
	return p
}

func PodWithAntiAffinity(pod *api.Pod, clusterName string) *api.Pod {
	// set pod anti-affinity with the pods that belongs to the same etcd cluster
	affinity := api.Affinity{
		PodAntiAffinity: &api.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []api.PodAffinityTerm{
				{
					LabelSelector: &unversionedAPI.LabelSelector{
						MatchLabels: map[string]string{
							"tidb_cluster": clusterName,
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}

	affinityb, err := json.Marshal(affinity)
	if err != nil {
		panic("failed to marshal affinty struct")
	}

	pod.Annotations[api.AffinityAnnotationKey] = string(affinityb)
	return pod
}

func CreateAndWaitPod(kclient *unversioned.Client, ns string, pod *api.Pod, timeout time.Duration) (*api.Pod, error) {
	if _, err := kclient.Pods(ns).Create(pod); err != nil {
		return nil, err
	}
	// TODO: cleanup pod on failure
	w, err := kclient.Pods(ns).Watch(api.SingleObject(api.ObjectMeta{Name: pod.Name}))
	if err != nil {
		return nil, err
	}
	_, err = watch.Until(timeout, w, unversioned.PodRunning)

	pod, err = kclient.Pods(ns).Get(pod.Name)
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func makeTiDBService(clusternName string) *api.Service {
	labels := map[string]string{
		"app":          "tidb",
		"tidb_clsuetr": clusternName,
	}

	svc := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:   clusterName,
			Labels: labels,
		},
		Spec: api.ServiceSpec{
			Ports: []api.ServicePort{
				{
					Name:       "mysql-client",
					Port:       4000,
					TargetPort: intstr.FromInt(4000),
					Protocol:   api.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
}

func makePDService(clusterName string) *api.Service {
	labels := map[string]string{
		"app":          "pd",
		"tidb_cluster": clusterName,
	}
	svc := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:   clusterName,
			Labels: labels,
		},
		Spec: api.ServiceSpec{
			Ports: []api.ServicePort{
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

func makePDNodePortService(pdName, clusterName string) *api.Service {
	labels := map[string]string{
		"pd_node":      pdName,
		"tidb_cluster": clusterName,
	}
	svc := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:   pdName + "-nodeport",
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
			Type:     api.ServiceTypeNodePort,
			Selector: labels,
		},
	}
	return svc
}

func makeTiDBNodePortService(tidbName, clusterName string) *api.Service {
	labels := map[string]string{
		"tidb_node":    tidbName,
		"tidb_cluster": clusterName,
	}
	svc := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:   tidbName + "-nodeport",
			Labels: labels,
		},
		Spec: api.ServiceSpec{
			Ports: []api.ServicePort{
				{
					Name:       "mysql-client",
					Port:       4000,
					TargetPort: intstr.FromInt(4000),
					Protocol:   api.ProtocolTCP,
				},
			},
			Type:     api.ServiceTypeNodePort,
			Selector: labels,
		},
	}
	return svc
}

func removeMember(clientURLs []string, id uint64, tp MemberType) error {
	cfg := clientv3.Config{
		Endpoints:   clientURLs,
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	switch {
	case PD:
		_, err = etcdcli.Cluster.MemberRemove(ctx, id)
	}
	return err
}

func PodWithNodeSelector(p *api.Pod, ns map[string]string) *api.Pod {
	p.Spec.NodeSelector = ns
	return p
}

func needUpgrade(pods []*api.Pod, cs *spec.ClusterSpec) bool {
	return len(pods) == cs.Size && pickOneOldMember(pods, cs.Version) != nil
}

func findID(name string) int {
	i := strings.LastIndex(name, "-")
	id, err := strconv.Atoi(name[i+1:])
	if err != nil {
		panic(fmt.Sprintf("TODO: fail to extract valid ID from name (%s): %v", name, err))
	}
	return id
}

func PodListOpt(clusterName string, tp MemberType) api.ListOptions {
	return api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"tidb_cluster": clusterName,
			"app":          tp.String(),
		}),
	}
}
