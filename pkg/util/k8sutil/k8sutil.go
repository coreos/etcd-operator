package k8sutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/kube-etcd-controller/pkg/util/etcdutil"
	"k8s.io/kubernetes/pkg/api"
	apierrors "k8s.io/kubernetes/pkg/api/errors"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
	k8sv1api "k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/pkg/watch"
)

const (
	etcdImage = "quay.io/coreos/etcd:v3.1.0-alpha.1"

	// TODO: This is constant for current purpose. We might make it configurable later.
	etcdDir    = "/var/etcd"
	dataDir    = etcdDir + "/data"
	backupFile = "/var/etcd/latest.backup"
)

func makeRestoreInitContainerSpec(backupAddr, name, token string) string {
	spec := []api.Container{
		{
			Name:  "fetch-backup",
			Image: "tutum/curl",
			Command: []string{
				"/bin/sh", "-c",
				fmt.Sprintf("curl -o %s http://%s/backup", backupFile, backupAddr),
			},
			VolumeMounts: []api.VolumeMount{
				{Name: "etcd-data", MountPath: etcdDir},
			},
		},
		{
			Name:  "restore-datadir",
			Image: etcdImage,
			Command: []string{
				"/bin/sh", "-c",
				fmt.Sprintf("ETCDCTL_API=3 etcdctl snapshot restore %[1]s"+
					" --name %[2]s"+
					" --initial-cluster %[2]s=http://%[2]s:2380"+
					" --initial-cluster-token %[3]s"+
					" --initial-advertise-peer-urls http://%[2]s:2380"+
					" --data-dir %[4]s", backupFile, name, token, dataDir),
			},
			VolumeMounts: []api.VolumeMount{
				{Name: "etcd-data", MountPath: etcdDir},
			},
		},
	}
	b, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func GetNodePortString(srv *api.Service) string {
	return fmt.Sprint(srv.Spec.Ports[0].NodePort)
}

func MakeBackupHostPort(clusterName string) string {
	return fmt.Sprintf("%s:19999", makeBackupName(clusterName))
}

func WithAddMemberInitContainer(p *api.Pod, endpoints []string, name string, peerURLs []string) *api.Pod {
	spec := []api.Container{
		{
			Name:  "add-member",
			Image: etcdImage,
			Command: []string{
				"/bin/sh", "-c",
				fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=%s member add %s --peer-urls=%s", strings.Join(endpoints, ","), name, strings.Join(peerURLs, ",")),
			},
		},
	}
	b, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	if p.Annotations == nil {
		p.Annotations = map[string]string{}
	}
	p.Annotations[k8sv1api.PodInitContainersAnnotationKey] = string(b)
	return p
}

func makeBackupName(clusterName string) string {
	return fmt.Sprintf("%s-backup-tool", clusterName)
}

func CreateEtcdService(kclient *unversioned.Client, etcdName, clusterName, ns string) error {
	svc := makeEtcdService(etcdName, clusterName)
	if _, err := kclient.Services(ns).Create(svc); err != nil {
		return err
	}
	return nil
}

func CreateEtcdNodePortService(kclient *unversioned.Client, etcdName, clusterName, ns string) (*api.Service, error) {
	svc := makeEtcdNodePortService(etcdName, clusterName)
	return kclient.Services(ns).Create(svc)
}

// TODO: use a struct to replace the huge arg list.
func CreateAndWaitPod(kclient *unversioned.Client, pod *api.Pod, m *etcdutil.Member, ns string) error {
	if _, err := kclient.Pods(ns).Create(pod); err != nil {
		return err
	}
	w, err := kclient.Pods(ns).Watch(api.SingleObject(api.ObjectMeta{Name: m.Name}))
	if err != nil {
		return err
	}
	_, err = watch.Until(100*time.Second, w, unversioned.PodRunningAndReady)
	// TODO: cleanup pod on failure
	return err
}

// TODO: converge the port logic with member ClientAddr() and PeerAddr()
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

func makeEtcdNodePortService(etcdName, clusterName string) *api.Service {
	labels := map[string]string{
		"etcd_node":    etcdName,
		"etcd_cluster": clusterName,
	}
	svc := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:   etcdName + "-nodeport",
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
			},
			Type:     api.ServiceTypeNodePort,
			Selector: labels,
		},
	}
	return svc
}

func AddRecoveryToPod(pod *api.Pod, clusterName, name, token string) {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[k8sv1api.PodInitContainersAnnotationKey] = makeRestoreInitContainerSpec(MakeBackupHostPort(clusterName), name, token)
}

// todo: use a struct to replace the huge arg list.
func MakeEtcdPod(m *etcdutil.Member, initialCluster []string, clusterName, state, token string, antiAffinity bool, hostNet bool) *api.Pod {
	commands := []string{
		"/usr/local/bin/etcd",
		"--data-dir",
		dataDir,
		"--name",
		m.Name,
		"--initial-advertise-peer-urls",
		m.PeerAddr(),
		"--listen-peer-urls",
		"http://0.0.0.0:2380",
		"--listen-client-urls",
		"http://0.0.0.0:2379",
		"--advertise-client-urls",
		m.ClientAddr(),
		"--initial-cluster",
		strings.Join(initialCluster, ","),
		"--initial-cluster-state",
		state,
	}
	if state == "new" {
		commands = append(commands, "--initial-cluster-token", token)
	}
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: m.Name,
			Labels: map[string]string{
				"app":          "etcd",
				"etcd_node":    m.Name,
				"etcd_cluster": clusterName,
			},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Command: commands,
					Name:    m.Name,
					Image:   etcdImage,
					Ports: []api.ContainerPort{
						{
							Name:          "server",
							ContainerPort: int32(2380),
							Protocol:      api.ProtocolTCP,
						},
					},
					// set pod to ready when a get succeeds
					ReadinessProbe: &api.Probe{
						Handler: api.Handler{
							Exec: &api.ExecAction{
								Command: []string{"/bin/sh", "-c",
									"ETCDCTL_API=3 etcdctl get foo"},
							},
						},
						// if an etcd member tries to join quorum, it has 5s strict check
						// before it can serve any client request
						InitialDelaySeconds: 7,
						TimeoutSeconds:      10,
						PeriodSeconds:       10,
						FailureThreshold:    3,
					},
					// a pod is alive only if a get succeeds
					// the etcd pod should die if liveness probing fails.
					LivenessProbe: &api.Probe{
						Handler: api.Handler{
							Exec: &api.ExecAction{
								Command: []string{"/bin/sh", "-c",
									"ETCDCTL_API=3 etcdctl get foo"},
							},
						},
						InitialDelaySeconds: 10,
						TimeoutSeconds:      10,
						// probe every 60 seconds
						PeriodSeconds: 60,
						// failed for 3 minutes
						FailureThreshold: 3,
					},
					VolumeMounts: []api.VolumeMount{
						{Name: "etcd-data", MountPath: etcdDir},
					},
				},
			},
			RestartPolicy: api.RestartPolicyNever,
			SecurityContext: &api.PodSecurityContext{
				HostNetwork: hostNet,
			},
			Volumes: []api.Volume{
				{Name: "etcd-data", VolumeSource: api.VolumeSource{EmptyDir: &api.EmptyDirVolumeSource{}}},
			},
		},
	}

	if !antiAffinity {
		return pod
	}

	// set pod anti-affinity with the pods that belongs to the same etcd cluster
	affinity := api.Affinity{
		PodAntiAffinity: &api.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []api.PodAffinityTerm{
				{
					LabelSelector: &unversionedAPI.LabelSelector{
						MatchLabels: map[string]string{
							"etcd_cluster": clusterName,
						},
					},
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

func MustGetInClusterMasterHost() string {
	cfg, err := restclient.InClusterConfig()
	if err != nil {
		panic(err)
	}
	return cfg.Host
}

func MustCreateClient(host string, tlsInsecure bool, tlsConfig *restclient.TLSClientConfig) *unversioned.Client {
	if len(host) == 0 {
		c, err := unversioned.NewInCluster()
		if err != nil {
			panic(err)
		}
		return c
	}
	cfg := &restclient.Config{
		Host:  host,
		QPS:   100,
		Burst: 100,
	}
	hostUrl, err := url.Parse(host)
	if err != nil {
		panic(fmt.Sprintf("error parsing host url %s : %v", host, err))
	}
	if hostUrl.Scheme == "https" {
		cfg.TLSClientConfig = *tlsConfig
		cfg.Insecure = tlsInsecure
	}
	c, err := unversioned.New(cfg)
	if err != nil {
		panic(err)
	}
	return c
}

func IsKubernetesResourceAlreadyExistError(err error) bool {
	se, ok := err.(*apierrors.StatusError)
	if !ok {
		return false
	}
	if se.Status().Code == http.StatusConflict && se.Status().Reason == unversionedAPI.StatusReasonAlreadyExists {
		return true
	}
	return false
}

func IsKubernetesResourceNotFoundError(err error) bool {
	se, ok := err.(*apierrors.StatusError)
	if !ok {
		return false
	}
	if se.Status().Code == http.StatusNotFound && se.Status().Reason == unversionedAPI.StatusReasonNotFound {
		return true
	}
	return false
}

func ListETCDCluster(host, ns string, httpClient *http.Client) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters",
		host, ns))
}

func WatchETCDCluster(host, ns string, httpClient *http.Client, resourceVersion string) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters?watch=true&resourceVersion=%s",
		host, ns, resourceVersion))
}

func WaitEtcdTPRReady(httpClient *http.Client, interval, timeout time.Duration, host, ns string) error {
	return wait.Poll(interval, timeout, func() (bool, error) {
		resp, err := ListETCDCluster(host, ns, httpClient)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			return true, nil
		case http.StatusNotFound: // not set up yet. wait.
			return false, nil
		default:
			return false, fmt.Errorf("invalid status code: %v", resp.Status)
		}
	})
}

func SliceReadyAndUnreadyPods(podList *api.PodList) (ready, unready []string) {
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == api.PodRunning && api.IsPodReady(pod) {
			ready = append(ready, pod.Name)
			continue
		}
		unready = append(unready, pod.Name)
	}
	return
}

func EtcdPodListOpt(clusterName string) api.ListOptions {
	return api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"etcd_cluster": clusterName,
			"app":          "etcd",
		}),
	}
}
