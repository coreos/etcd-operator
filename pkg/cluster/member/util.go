package member

import (
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
	versionAnnotationKeys = map[ServiceType]string{
		TiDB: "tidb.version",
		TiKV: "tikv.version",
		PD:   "pd.version",
	}
	imageNames = map[ServiceType]string{
		TiDB: "pingcap/tidb",
		TiKV: "pingcap/tikv",
		PD:   "pingcap/pd",
	}
)

func MakeImage(version string, tp MemberType) string {
	return fmt.Sprintf("%s:%v", imageNames[tp], version)
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
