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
