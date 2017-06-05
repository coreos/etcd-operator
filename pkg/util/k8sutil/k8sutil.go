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
	"net"
	"os"
	"strings"
	"time"

	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // for gcp auth
	"k8s.io/client-go/rest"
)

const (
	etcdVolumeMountDir       = "/var/etcd"
	dataDir                  = etcdVolumeMountDir + "/data"
	backupFile               = "/var/etcd/latest.backup"
	etcdVersionAnnotationKey = "etcd.version"
	peerTLSDir               = "/etc/etcdtls/member/peer-tls"
	peerTLSVolume            = "member-peer-tls"
	clientTLSDir             = "/etc/etcdtls/member/client-tls"
	clientTLSVolume          = "member-client-tls"
	operatorEtcdTLSDir       = "/etc/etcdtls/operator/etcd-tls"
	operatorEtcdTLSVolume    = "operator-etcd-tls"
)

func GetEtcdVersion(pod *v1.Pod) string {
	return pod.Annotations[etcdVersionAnnotationKey]
}

func SetEtcdVersion(pod *v1.Pod, version string) {
	pod.Annotations[etcdVersionAnnotationKey] = version
}

func GetPodNames(pods []*v1.Pod) []string {
	if len(pods) == 0 {
		return nil
	}
	res := []string{}
	for _, p := range pods {
		res = append(res, p.Name)
	}
	return res
}

func makeRestoreInitContainerSpec(backupAddr, token, version string, m *etcdutil.Member) string {
	spec := []v1.Container{
		{
			Name:  "fetch-backup",
			Image: "tutum/curl",
			Command: []string{
				"/bin/sh", "-ec",
				fmt.Sprintf("curl -o %s %s", backupFile, backupapi.NewBackupURL("http", backupAddr, version, -1)),
			},
			VolumeMounts: etcdVolumeMounts(),
		},
		{
			Name:  "restore-datadir",
			Image: EtcdImageName(version),
			Command: []string{
				"/bin/sh", "-ec",
				fmt.Sprintf("ETCDCTL_API=3 etcdctl snapshot restore %[1]s"+
					" --name %[2]s"+
					" --initial-cluster %[2]s=%[3]s"+
					" --initial-cluster-token %[4]s"+
					" --initial-advertise-peer-urls %[3]s"+
					" --data-dir %[5]s", backupFile, m.Name, m.PeerURL(), token, dataDir),
			},
			VolumeMounts: etcdVolumeMounts(),
		},
	}
	b, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func EtcdImageName(version string) string {
	return fmt.Sprintf("quay.io/coreos/etcd:v%v", version)
}
func PodWithNodeSelector(p *v1.Pod, ns map[string]string) *v1.Pod {
	p.Spec.NodeSelector = ns
	return p
}

func CreateClientService(kubecli kubernetes.Interface, clusterName, ns string, owner metav1.OwnerReference) error {
	return createService(kubecli, ClientServiceName(clusterName), clusterName, ns, "", 2379, owner)
}

func ClientServiceName(clusterName string) string {
	return clusterName + "-client"
}

func CreatePeerService(kubecli kubernetes.Interface, clusterName, ns string, owner metav1.OwnerReference) error {
	return createService(kubecli, clusterName, clusterName, ns, v1.ClusterIPNone, 2380, owner)
}

func createService(kubecli kubernetes.Interface, svcName, clusterName, ns, clusterIP string, port int32, owner metav1.OwnerReference) error {
	svc := newEtcdServiceManifest(svcName, clusterName, clusterIP, port)
	addOwnerRefToObject(svc.GetObjectMeta(), owner)
	_, err := kubecli.CoreV1().Services(ns).Create(svc)
	return err
}

// CreateAndWaitPod is a workaround for self hosted and util for testing.
// We should eventually get rid of this in critical code path and move it to test util.
func CreateAndWaitPod(kubecli kubernetes.Interface, ns string, pod *v1.Pod, timeout time.Duration) (*v1.Pod, error) {
	_, err := kubecli.CoreV1().Pods(ns).Create(pod)
	if err != nil {
		return nil, err
	}

	interval := 3 * time.Second
	var retPod *v1.Pod
	err = retryutil.Retry(interval, int(timeout/(interval)), func() (bool, error) {
		retPod, err = kubecli.CoreV1().Pods(ns).Get(pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch retPod.Status.Phase {
		case v1.PodRunning:
			return true, nil
		case v1.PodPending:
			return false, nil
		default:
			return false, fmt.Errorf("unexpected pod status.phase: %v", retPod.Status.Phase)
		}
	})

	return retPod, err
}

func newEtcdServiceManifest(svcName, clusterName string, clusterIP string, port int32) *v1.Service {
	labels := map[string]string{
		"app":          "etcd",
		"etcd_cluster": clusterName,
	}
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   svcName,
			Labels: labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "client",
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector:  labels,
			ClusterIP: clusterIP,
		},
	}
	return svc
}

func AddRecoveryToPod(pod *v1.Pod, clusterName, token string, m *etcdutil.Member, cs spec.ClusterSpec) {
	pod.Annotations[v1.PodInitContainersBetaAnnotationKey] =
		makeRestoreInitContainerSpec(BackupServiceAddr(clusterName), token, cs.Version, m)
}

func addOwnerRefToObject(o metav1.Object, r metav1.OwnerReference) {
	o.SetOwnerReferences(append(o.GetOwnerReferences(), r))
}

func NewEtcdPod(m *etcdutil.Member, initialCluster []string, clusterName, state, token string, cs spec.ClusterSpec, owner metav1.OwnerReference) *v1.Pod {
	commands := fmt.Sprintf("/usr/local/bin/etcd --data-dir=%s --name=%s --initial-advertise-peer-urls=%s "+
		"--listen-peer-urls=%s --listen-client-urls=%s --advertise-client-urls=%s "+
		"--initial-cluster=%s --initial-cluster-state=%s",
		dataDir, m.Name, m.PeerURL(), m.ListenPeerURL(), m.ListenClientURL(), m.ClientAddr(), strings.Join(initialCluster, ","), state)
	if m.SecurePeer {
		commands += fmt.Sprintf(" --peer-client-cert-auth=true --peer-trusted-ca-file=%[1]s/peer-ca-crt.pem --peer-cert-file=%[1]s/peer-crt.pem --peer-key-file=%[1]s/peer-key.pem", peerTLSDir)
	}
	if m.SecureClient {
		commands += fmt.Sprintf(" --client-cert-auth=true --trusted-ca-file=%[1]s/client-ca-crt.pem --cert-file=%[1]s/client-crt.pem --key-file=%[1]s/client-key.pem", clientTLSDir)
	}
	if state == "new" {
		commands = fmt.Sprintf("%s --initial-cluster-token=%s", commands, token)
	}

	labels := map[string]string{
		"app":          "etcd",
		"etcd_node":    m.Name,
		"etcd_cluster": clusterName,
	}

	container := containerWithLivenessProbe(etcdContainer(commands, cs.Version), etcdLivenessProbe(cs.TLS.IsSecureClient()))
	if cs.Pod != nil {
		container = containerWithRequirements(container, cs.Pod.Resources)
	}

	volumes := []v1.Volume{
		{Name: "etcd-data", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
	}

	if m.SecurePeer {
		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			MountPath: peerTLSDir,
			Name:      peerTLSVolume,
		})
		volumes = append(volumes, v1.Volume{Name: peerTLSVolume, VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{SecretName: cs.TLS.Static.Member.PeerSecret},
		}})
	}
	if m.SecureClient {
		container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
			MountPath: clientTLSDir,
			Name:      clientTLSVolume,
		}, v1.VolumeMount{
			MountPath: operatorEtcdTLSDir,
			Name:      operatorEtcdTLSVolume,
		})
		volumes = append(volumes, v1.Volume{Name: clientTLSVolume, VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{SecretName: cs.TLS.Static.Member.ClientSecret},
		}}, v1.Volume{Name: operatorEtcdTLSVolume, VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{SecretName: cs.TLS.Static.OperatorSecret},
		}})
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        m.Name,
			Labels:      labels,
			Annotations: map[string]string{},
		},
		Spec: v1.PodSpec{
			Containers:    []v1.Container{container},
			RestartPolicy: v1.RestartPolicyNever,
			Volumes:       volumes,
			// DNS A record: [m.Name].[clusterName].Namespace.svc.cluster.local.
			// For example, etcd-0000 in default namesapce will have DNS name
			// `etcd-0000.etcd.default.svc.cluster.local`.
			Hostname:  m.Name,
			Subdomain: clusterName,
		},
	}

	applyPodPolicy(clusterName, pod, cs.Pod)

	SetEtcdVersion(pod, cs.Version)

	addOwnerRefToObject(pod.GetObjectMeta(), owner)
	return pod
}

func MustNewKubeClient() kubernetes.Interface {
	cfg, err := InClusterConfig()
	if err != nil {
		panic(err)
	}
	return kubernetes.NewForConfigOrDie(cfg)
}

func InClusterConfig() (*rest.Config, error) {
	// Work around https://github.com/kubernetes/kubernetes/issues/40973
	// See https://github.com/coreos/etcd-operator/issues/731#issuecomment-283804819
	if len(os.Getenv("KUBERNETES_SERVICE_HOST")) == 0 {
		addrs, err := net.LookupHost("kubernetes.default.svc")
		if err != nil {
			panic(err)
		}
		os.Setenv("KUBERNETES_SERVICE_HOST", addrs[0])
	}
	if len(os.Getenv("KUBERNETES_SERVICE_PORT")) == 0 {
		os.Setenv("KUBERNETES_SERVICE_PORT", "443")
	}
	return rest.InClusterConfig()
}

func NewTPRClient() (*rest.RESTClient, error) {
	config, err := InClusterConfig()
	if err != nil {
		return nil, err
	}

	config.GroupVersion = &schema.GroupVersion{
		Group:   spec.TPRGroup,
		Version: spec.TPRVersion,
	}
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}

	restcli, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}
	return restcli, nil
}

func IsKubernetesResourceAlreadyExistError(err error) bool {
	return apierrors.IsAlreadyExists(err)
}

func IsKubernetesResourceNotFoundError(err error) bool {
	return apierrors.IsNotFound(err)
}

// We are using internal api types for cluster related.
func ClusterListOpt(clusterName string) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(LabelsForCluster(clusterName)).String(),
	}
}

func LabelsForCluster(clusterName string) map[string]string {
	return map[string]string{
		"etcd_cluster": clusterName,
		"app":          "etcd",
	}
}

func CreatePatch(o, n, datastruct interface{}) ([]byte, error) {
	oldData, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}
	newData, err := json.Marshal(n)
	if err != nil {
		return nil, err
	}
	return strategicpatch.CreateTwoWayMergePatch(oldData, newData, datastruct)
}

func ClonePod(p *v1.Pod) *v1.Pod {
	np, err := api.Scheme.DeepCopy(p)
	if err != nil {
		panic("cannot deep copy pod")
	}
	return np.(*v1.Pod)
}

func cloneDeployment(d *appsv1beta1.Deployment) *appsv1beta1.Deployment {
	cd, err := api.Scheme.DeepCopy(d)
	if err != nil {
		panic("cannot deep copy pod")
	}
	return cd.(*appsv1beta1.Deployment)
}

func PatchDeployment(kubecli kubernetes.Interface, namespace, name string, updateFunc func(*appsv1beta1.Deployment)) error {
	od, err := kubecli.AppsV1beta1().Deployments(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	nd := cloneDeployment(od)
	updateFunc(nd)
	patchData, err := CreatePatch(od, nd, appsv1beta1.Deployment{})
	if err != nil {
		return err
	}
	_, err = kubecli.AppsV1beta1().Deployments(namespace).Patch(name, types.StrategicMergePatchType, patchData)
	return err
}

func CascadeDeleteOptions(gracePeriodSeconds int64) *metav1.DeleteOptions {
	return &metav1.DeleteOptions{
		GracePeriodSeconds: func(t int64) *int64 { return &t }(gracePeriodSeconds),
		PropagationPolicy: func() *metav1.DeletionPropagation {
			foreground := metav1.DeletePropagationForeground
			return &foreground
		}(),
	}
}

// mergeLables merges l2 into l1. Conflicting label will be skipped.
func mergeLabels(l1, l2 map[string]string) {
	for k, v := range l2 {
		if _, ok := l1[k]; ok {
			continue
		}
		l1[k] = v
	}
}
