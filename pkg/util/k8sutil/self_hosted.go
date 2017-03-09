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
	"strings"

	"github.com/coreos/etcd-operator/pkg/spec"

	"k8s.io/client-go/pkg/api/meta/metatypes"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	shouldCheckpointAnnotation = "checkpointer.alpha.coreos.com/checkpoint" // = "true"
)

var (
	envPodIP = v1.EnvVar{
		Name: "MY_POD_IP",
		ValueFrom: &v1.EnvVarSource{
			FieldRef: &v1.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "status.podIP",
			},
		},
	}
)

func PodWithAddMemberInitContainer(p *v1.Pod, endpoints []string, name string, peerURLs []string, cs spec.ClusterSpec) *v1.Pod {
	containerSpec := []v1.Container{
		{
			Name:  "add-member",
			Image: EtcdImageName(cs.Version),
			Command: []string{
				"/bin/sh", "-c",
				fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=%s member add %s --peer-urls=%s", strings.Join(endpoints, ","), name, strings.Join(peerURLs, ",")),
			},
			Env: []v1.EnvVar{envPodIP},
		},
	}
	b, err := json.Marshal(containerSpec)
	if err != nil {
		panic(err)
	}
	p.Annotations[v1.PodInitContainersBetaAnnotationKey] = string(b)
	return p
}

func NewSelfHostedEtcdPod(name string, initialCluster []string, clusterName, clusterNamespace, state, token string, cs spec.ClusterSpec, owner metatypes.OwnerReference) *v1.Pod {
	commands := fmt.Sprintf("/usr/local/bin/etcd --data-dir=%[1]s --name=%[2]s --initial-advertise-peer-urls=https://%[5]s.%[6]s.svc.cluster.local::2380 "+
		"--listen-peer-urls=https://$(MY_POD_IP):2380 --listen-client-urls=https://$(MY_POD_IP):2379 --advertise-client-urls=https://%[5]s.%[6]s.svc.cluster.local:2379 "+
		"--initial-cluster=%[3]s --initial-cluster-state=%[4]s",
		dataDir, name, strings.Join(initialCluster, ","), state, clusterName, clusterNamespace)

	if state == "new" {
		commands = fmt.Sprintf("%s --initial-cluster-token=%s", commands, token)
	}

	c := etcdContainer(commands, cs.Version)
	if cs.Pod != nil {
		c = containerWithRequirements(c, cs.Pod.Resources)
	}
	c.Env = []v1.EnvVar{envPodIP}
	pod := &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app":          "etcd",
				"etcd_node":    name,
				"etcd_cluster": clusterName,
			},
			Annotations: map[string]string{
				shouldCheckpointAnnotation: "true",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{c},
			// Self-hosted etcd pod need to endure node restart.
			// If we set it to Never, the pod won't restart. If etcd won't come up, nor does other k8s API components.
			RestartPolicy: v1.RestartPolicyAlways,
			HostNetwork:   true,
			Volumes: []v1.Volume{
				{Name: "etcd-data", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
				{Name: "etcd-node-tls", VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
					SecretName: cs.ClusterTLS.Static.NodeSecretName,
				}}},
				{Name: "etcd-client-tls", VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
					SecretName: cs.ClusterTLS.Static.ClientSecretName,
				}}},
				{Name: "etcd-operator-ca", VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
					SecretName: cs.ClusterTLS.Static.CASecretName,
				}}},
			},
		},
	}

	SetEtcdVersion(pod, cs.Version)

	pod = PodWithAntiAffinity(pod, clusterName)
	if cs.Pod != nil && len(cs.Pod.NodeSelector) != 0 {
		pod = PodWithNodeSelector(pod, cs.Pod.NodeSelector)
	}
	addOwnerRefToObject(pod.GetObjectMeta(), owner)
	return pod
}
