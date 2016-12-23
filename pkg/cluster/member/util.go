package member

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
