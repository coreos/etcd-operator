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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
)

func etcdVolumeMounts() []v1.VolumeMount {
	return []v1.VolumeMount{
		{Name: "etcd-data", MountPath: etcdVolumeMountDir},
	}
}

func etcdContainer(commands, version string) v1.Container {
	c := v1.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		Command: []string{"/bin/sh", "-ec", fmt.Sprintf("sleep 5; %s", commands)},
		Name:    "etcd",
		Image:   EtcdImageName(version),
		Ports: []v1.ContainerPort{
			{
				Name:          "server",
				ContainerPort: int32(2380),
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "client",
				ContainerPort: int32(2379),
				Protocol:      v1.ProtocolTCP,
			},
		},
		VolumeMounts: etcdVolumeMounts(),
	}

	return c
}

func containerWithLivenessProbe(c v1.Container, lp *v1.Probe) v1.Container {
	c.LivenessProbe = lp
	return c
}

func containerWithRequirements(c v1.Container, r v1.ResourceRequirements) v1.Container {
	c.Resources = r
	return c
}

func etcdLivenessProbe() *v1.Probe {
	// etcd pod is alive only if a linearizable get succeeds.
	return &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{"/bin/sh", "-ec",
					"ETCDCTL_API=3 etcdctl get foo"},
			},
		},
		InitialDelaySeconds: 10,
		TimeoutSeconds:      10,
		PeriodSeconds:       60,
		FailureThreshold:    3,
	}
}

func PodWithAntiAffinity(pod *v1.Pod, clusterName string) *v1.Pod {
	// set pod anti-affinity with the pods that belongs to the same etcd cluster
	affinity := v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"etcd_cluster": clusterName,
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
