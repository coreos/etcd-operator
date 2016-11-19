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

	"k8s.io/kubernetes/pkg/api"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
)

var (
	envPodIP = api.EnvVar{
		Name: "MY_POD_IP",
		ValueFrom: &api.EnvVarSource{
			FieldRef: &api.ObjectFieldSelector{
				FieldPath: "status.podIP",
			},
		},
	}
)

func etcdContainer(commands, version string) api.Container {
	c := api.Container{
		// TODO: fix "sleep 5".
		// Without waiting some time, there is highly probable flakes in network setup.
		Command: []string{"/bin/sh", "-c", fmt.Sprintf("sleep 5; %s", commands)},
		Name:    "etcd",
		Image:   MakeEtcdImage(version),
		Ports: []api.ContainerPort{
			{
				Name:          "server",
				ContainerPort: int32(2380),
				Protocol:      api.ProtocolTCP,
			},
			{
				Name:          "client",
				ContainerPort: int32(2379),
				Protocol:      api.ProtocolTCP,
			},
		},
		// a pod is alive only if a get succeeds
		// the etcd pod should die if liveness probing fails.
		LivenessProbe: &api.Probe{
			Handler: api.Handler{
				Exec: &api.ExecAction{
					Command: []string{"/bin/sh", "-c",
						"ETCDCTL_API=3 etcdctl --endpoints=http://$(MY_POD_IP):2379 get foo"},
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
		Env: []api.EnvVar{envPodIP},
	}

	return c
}

func PodWithAntiAffinity(pod *api.Pod, clusterName string) *api.Pod {
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
