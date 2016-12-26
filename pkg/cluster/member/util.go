package member

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GregoryIan/operator/pkg/spec"
	"github.com/juju/errors"
	"k8s.io/kubernetes/pkg/api"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
	k8sv1api "k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/watch"
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

func SplitAndDistributeSpec(news, olds *spec.ClusterSpec, ms map[MemberType]MemberSet) {
	for _, tp := range ServiceAdjustSequence {
		switch tp {
		case PD:
			if news.PD != nil {
				olds.PD = news.PD
				ms[PD].SetSpec(news.PD)
			}
		}
	}
}

func IsSpecChange(s *spec.ClusterSpec, ms map[MemberType]MemberSet) bool {
	for _, tp := range ServiceAdjustSequence {
		switch tp {
		case PD:
			if s.PD != nil {
				sp := ms[tp].GetSpec()
				if len(s.PD.Version) == 0 {
					s.PD.Version = defaultVersion
				}
				if sp.Size != s.PD.Size || sp.Version != s.PD.Version {
					return true
				}
			}
		}
	}
	return false
}

func IsNonConsistent(size, localSize int, tp MemberType) bool {
	switch tp {
	case PD:
		return size < localSize/2+1
	}

	return true
}

func GetSpecVersion(s *spec.ClusterSpec, tp MemberType) string {
	switch tp {
	case PD:
		return s.PD.Version
	}

	return ""
}

func GetSpecSize(s *spec.ClusterSpec, tp MemberType) int {
	switch tp {
	case PD:
		return s.PD.Size
	}

	return 0
}

func GetVersion(pod *api.Pod, tp MemberType) string {
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

func CreatePDService(kclient *unversioned.Client, clusterName, ns string) (*api.Service, error) {
	svc := makePDService(clusterName)
	retSvc, err := kclient.Services(ns).Create(svc)
	if err != nil {
		return nil, err
	}
	return retSvc, nil
}

func PodWithAddMemberInitContainer(p *api.Pod, endpoints []string, name string, peerURLs []string, cs *spec.ServiceSpec) *api.Pod {
	containerSpec := []api.Container{
		{
			Name:  "add-member",
			Image: MakeImage(cs.Version, PD),
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
		return nil, errors.Trace(err)
	}
	// TODO: cleanup pod on failure
	w, err := kclient.Pods(ns).Watch(api.SingleObject(api.ObjectMeta{Name: pod.Name}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	_, err = watch.Until(timeout, w, unversioned.PodRunning)

	pod, err = kclient.Pods(ns).Get(pod.Name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return pod, nil
}

func makeTiDBService(clusterName string) *api.Service {
	labels := map[string]string{
		"app":          "tidb",
		"tidb_clsuetr": clusterName,
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

	return svc
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

func RemoveMember(clientURLs []string, id uint64, tp MemberType) error {
	/*cfg := clientv3.Config{
		Endpoints:   clientURLs,
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return errors.Trace(err)
	}
	defer etcdcli.Close()

	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	switch {
	case PD:
		_, err = etcdcli.Cluster.MemberRemove(ctx, id)
	}
	return errors.Trace(err)*/
	return nil
}

func PodWithNodeSelector(p *api.Pod, ns map[string]string) *api.Pod {
	p.Spec.NodeSelector = ns
	return p
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
