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

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	etcdVolumeName = "etcd-data"
)

func etcdVolumeMounts() []v1.VolumeMount {
	return []v1.VolumeMount{
		{Name: etcdVolumeName, MountPath: etcdVolumeMountDir},
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

func etcdLivenessProbe(isSecure bool) *v1.Probe {
	// etcd pod is alive only if a linearizable get succeeds.
	cmd := "ETCDCTL_API=3 etcdctl get foo"
	if isSecure {
		tlsFlags := fmt.Sprintf("--cert=%[1]s/%[2]s --key=%[1]s/%[3]s --cacert=%[1]s/%[4]s", operatorEtcdTLSDir, etcdutil.CliCertFile, etcdutil.CliKeyFile, etcdutil.CliCAFile)
		cmd = fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=https://localhost:2379 %s get foo", tlsFlags)
	}
	return &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{"/bin/sh", "-ec", cmd},
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
	ls := &metav1.LabelSelector{MatchLabels: map[string]string{
		"etcd_cluster": clusterName,
	}}
	return podWithAntiAffinity(pod, ls)
}

func podWithAntiAffinity(pod *v1.Pod, ls *metav1.LabelSelector) *v1.Pod {
	affinity := &v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: ls,
					TopologyKey:   "kubernetes.io/hostname",
				},
			},
		},
	}

	pod.Spec.Affinity = affinity
	return pod
}

func applyPodPolicy(clusterName string, pod *v1.Pod, policy *spec.PodPolicy) {
	if policy == nil {
		return
	}

	if policy.AntiAffinity {
		pod = PodWithAntiAffinity(pod, clusterName)
	}

	if len(policy.NodeSelector) != 0 {
		pod = PodWithNodeSelector(pod, policy.NodeSelector)
	}
	if len(policy.Tolerations) != 0 {
		pod.Spec.Tolerations = policy.Tolerations
	}

	mergeLabels(pod.Labels, policy.Labels)

	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == "etcd" {
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, policy.EtcdEnv...)
		}
	}
}

// only used for backup pod.
func applyPodPolicyToPodTemplateSpec(clusterName string, pod *v1.PodTemplateSpec, policy *spec.PodPolicy) {
	if policy == nil {
		return
	}

	// TODO: anti-affinity for backup pod?

	if len(policy.NodeSelector) != 0 {
		pod.Spec.NodeSelector = policy.NodeSelector
	}
	if len(policy.Tolerations) != 0 {
		pod.Spec.Tolerations = policy.Tolerations
	}

	mergeLabels(pod.Labels, policy.Labels)
}
