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
	"fmt"
	"strings"

	"github.com/coreos/etcd-operator/pkg/spec"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/meta/metatypes"
)

const (
	shouldCheckpointAnnotation = "checkpointer.alpha.coreos.com/checkpoint" // = "true"
)

func MakeSelfHostedEtcdPod(name string, initialCluster []string, clusterName, state, token string, cs *spec.ClusterSpec, owner metatypes.OwnerReference) *api.Pod {
	commands := fmt.Sprintf("/usr/local/bin/etcd --data-dir=%s --name=%s --initial-advertise-peer-urls=http://$(MY_POD_IP):2380 "+
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
				"app":          "etcd",
				"etcd_node":    name,
				"etcd_cluster": clusterName,
			},
			Annotations: map[string]string{
				shouldCheckpointAnnotation: "true",
			},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				etcdContainer(commands, cs),
			},
			// Self-hosted etcd pod need to endure node restart.
			// If we set it to Never, the pod won't restart. If etcd won't come up, nor does other k8s API components.
			RestartPolicy: api.RestartPolicyAlways,
			SecurityContext: &api.PodSecurityContext{
				HostNetwork: true,
			},
			Volumes: []api.Volume{
				{Name: "etcd-data", VolumeSource: api.VolumeSource{EmptyDir: &api.EmptyDirVolumeSource{}}},
			},
		},
	}

	SetEtcdVersion(pod, cs.Version)

	pod = PodWithAntiAffinity(pod, clusterName)

	if len(cs.NodeSelector) != 0 {
		pod = PodWithNodeSelector(pod, cs.NodeSelector)
	}
	addOwnerRefToObject(pod.GetObjectMeta(), owner)
	return pod
}
