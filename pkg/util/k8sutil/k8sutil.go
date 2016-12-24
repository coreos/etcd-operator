// Copyright 2016 The etcd-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8sutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GregoryIan/operator/pkg/spec"
	"github.com/GregoryIan/operator/pkg/util"
	"github.com/GregoryIan/operator/pkg/util/constants"
	"github.com/GregoryIan/operator/pkg/util/etcdutil"

	"github.com/juju/errors"
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

func (s ServiceType) String() string {
	switch s {
	case TiDB:
		return "tidb"
	case TiKV:
		return "tikv"
	case PD:
		return "pd"
	}
}

const (
	// TODO: make it configurable, change it to others
	tidbDir                    = "/var/tidb-cluster"
	dataDir                    = etcdDir + "/data"
	pdVersionAnnotationKey     = "pd.version"
	tidbVersionAnnotationKey   = "tidb.version"
	tikvVersionAnnotationKey   = "tikv.version"
	annotationPrometheusScrape = "prometheus.io/scrape"
	annotationPrometheusPort   = "prometheus.io/port"
	versionAnnotationKeys      = map[ServiceType]string{
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

func GetVersion(pod *api.Pod, tp ServiceType) string {
	return pod.Annotations[versionAnnotationKeys[tp]]
}

func SetVersion(pod *api.Pod, version string, tp ServiceType) {
	pod.Annotations[versionAnnotationKeys[tp]] = version
}

func GetPodNames(pods []*api.Pod) []string {
	res := []string{}
	for _, p := range pods {
		res = append(res, p.Name)
	}
	return res
}

func MakeImage(version string, tp ServiceType) string {
	return fmt.Sprintf("%s:%v", imageNames[tp], version)
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

func PodWithNodeSelector(p *api.Pod, ns map[string]string) *api.Pod {
	p.Spec.NodeSelector = ns
	return p
}

func CreateService(kclient *unversioned.Client, clusterName, ns string, tp ServiceType) (*api.Service, error) {
	var svc *api.Service

	if tp == PD {
		svc = makePDService(clusterName)
	} else if tp == TiDB {
		svc = makeTiDBService(clusterName)
	} else {
		return errors.Errorf("%s can't make service", tp)
	}

	retSvc, err := kclient.Services(ns).Create(svc)
	if err != nil {
		return nil, err
	}
	return retSvc, nil
}

func CreateTiDBNodePortService(kclient *unversioned.Client, tidbName, clusterName, ns string) (*api.Service, error) {
	svc := makeTiDBNodePortService(tidbName, clusterName)
	return kclient.Services(ns).Create(svc)
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

func MakeSelfHostedEtcdPod(name string, initialCluster []string, clusterName, state, token string, cs *spec.ClusterSpec) *api.Pod {
	commands := fmt.Sprintf("/bin/pd --data-dir=%s --name=%s --initial-advertise-peer-urls=http://$(MY_POD_IP):2380 "+
		"--listen-peer-urls=http://$(MY_POD_IP):2380 --listen-client-urls=http://$(MY_POD_IP):2379 --advertise-client-urls=http://$(MY_POD_IP):2379 "+
		"--initial-cluster=%s --initial-cluster-state=%s",
		dataDir, name, strings.Join(initialCluster, ","), state)

	if state == "new" {
		commands = fmt.Sprintf("%s --initial-cluster-token=%s", commands, token)
	}

	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app":          "pd",
				"pd-node":      name,
				"tidb_cluster": clusterName,
			},
			Annotations: map[string]string{},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				etcdContainer(commands, cs.Pd.Version),
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

	SetVersion(pod, cs.Pd.Version, pd)

	pod = PodWithAntiAffinity(pod, clusterName)

	if len(cs.NodeSelector) != 0 {
		pod = PodWithNodeSelector(pod, cs.NodeSelector)
	}

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

func MustGetInClusterMasterHost() string {
	cfg, err := restclient.InClusterConfig()
	if err != nil {
		panic(err)
	}
	return cfg.Host
}

// tlsConfig isn't modified inside this function.
// The reason it's a pointer is that it's not necessary to have tlsconfig to create a client.
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

func ListTiDBCluster(host, ns string, httpClient *http.Client) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/pingcap.com/v1/namespaces/%s/tidbclusters",
		host, ns))
}

func WatchTiDBCluster(host, ns string, httpClient *http.Client, resourceVersion string) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/pingcap.com/v1/namespaces/%s/tidbclusters?watch=true&resourceVersion=%s",
		host, ns, resourceVersion))
}

func WaitTiDBTPRReady(httpClient *http.Client, interval, timeout time.Duration, host, ns string) error {
	return wait.Poll(interval, timeout, func() (bool, error) {
		resp, err := ListTiDBCluster(host, ns, httpClient)
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

func PodListOpt(clusterName string, op ServiceType) api.ListOptions {
	return api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"tidb_cluster": clusterName,
			"app":          op.String(),
		}),
	}
}
