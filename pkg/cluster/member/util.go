package member

import (
	"strconv"
	"strings"
)

var (
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

func needUpgrade(pods []*api.Pod, cs *spec.ClusterSpec) bool {
	return len(pods) == cs.Size && pickOneOldMember(pods, cs.Version) != nil
}

func pickOneOldMember(pods []*api.Pod, newVersion string) *etcdutil.Member {
	for _, pod := range pods {
		if k8sutil.GetEtcdVersion(pod) == newVersion {
			continue
		}
		return &etcdutil.Member{Name: pod.Name}
	}
	return nil
}

func findID(name string) int {
	i := strings.LastIndex(name, "-")
	id, err := strconv.Atoi(name[i+1:])
	if err != nil {
		panic(fmt.Sprintf("TODO: fail to extract valid ID from name (%s): %v", name, err))
	}
	return id
}

func MakePDPod(m *etcdutil.Member, initialCluster []string, clusterName, state, token string, cs *spec.ClusterSpec) *api.Pod {
	commands := fmt.Sprintf("/pd --data-dir=%s --name=%s --initial-advertise-peer-urls=%s "+
		"--listen-peer-urls=http://0.0.0.0:2380 --listen-client-urls=http://0.0.0.0:2379 --advertise-client-urls=%s "+
		"--initial-cluster=%s --initial-cluster-state=%s",
		dataDir, m.Name, m.PeerAddr(), m.ClientAddr(), strings.Join(initialCluster, ","), state)
	if state == "new" {
		commands = fmt.Sprintf("%s --initial-cluster-token=%s", commands, token)
	}
	//todo: check living
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: m.Name,
			Labels: map[string]string{
				"app":          "pd",
				"pd_node":      m.Name,
				"tidb_cluster": clusterName,
			},
			Annotations: map[string]string{},
		},
		Spec: api.PodSpec{
			Containers:    []api.Container{container},
			RestartPolicy: api.RestartPolicyNever,
			Volumes: []api.Volume{
				{Name: "pd-data", VolumeSource: api.VolumeSource{EmptyDir: &api.EmptyDirVolumeSource{}}},
			},
		},
	}

	SetVersion(pod, cs.Version, PD)
	if cs.AntiAffinity {
		pod = PodWithAntiAffinity(pod, clusterName)
	}

	if len(cs.NodeSelector) != 0 {
		pod = PodWithNodeSelector(pod, cs.NodeSelector)
	}

	return pod
}
