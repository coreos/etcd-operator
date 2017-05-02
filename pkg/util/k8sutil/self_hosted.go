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
	"path"
	"strings"

	"github.com/coreos/etcd-operator/pkg/spec"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	shouldCheckpointAnnotation = "checkpointer.alpha.coreos.com/checkpoint" // = "true"
	varLockVolumeName          = "var-lock"
	varLockDir                 = "/var/lock"
	etcdLockPath               = "/var/lock/etcd.lock"
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

func selfHostedDataDir(ns, name string) string {
	return path.Join(etcdVolumeMountDir, ns+"-"+name)
}

func PodWithAddMemberInitContainer(p *v1.Pod, endpoints []string, ns, name, peerURL string, cs spec.ClusterSpec) *v1.Pod {
	containerSpec := []v1.Container{
		{
			Name:  "add-member",
			Image: EtcdImageName(cs.Version),
			Command: []string{
				// NOTE: Init container will be re-executed on restart. We are taking the datadir as a signal of restart.
				"/bin/sh", "-ec",
				fmt.Sprintf("[ -d %s ] || ETCDCTL_API=3 etcdctl --endpoints=%s member add %s --peer-urls=%s",
					selfHostedDataDir(ns, name), strings.Join(endpoints, ","), name, peerURL),
			},
			Env:          []v1.EnvVar{envPodIP},
			VolumeMounts: etcdVolumeMounts(),
		},
	}
	p.Spec.InitContainers = containerSpec
	return p
}

func NewSelfHostedEtcdPod(name string, initialCluster []string, clusterName, ns, state, token string, cs spec.ClusterSpec, owner metav1.OwnerReference) *v1.Pod {
	commands := fmt.Sprintf("/usr/local/bin/etcd --data-dir=%s --name=%s --initial-advertise-peer-urls=http://$(MY_POD_IP):2380 "+
		"--listen-peer-urls=http://$(MY_POD_IP):2380 --listen-client-urls=http://$(MY_POD_IP):2379 --advertise-client-urls=http://$(MY_POD_IP):2379 "+
		"--initial-cluster=%s --initial-cluster-state=%s --metrics extensive",
		selfHostedDataDir(ns, name), name, strings.Join(initialCluster, ","), state)

	if state == "new" {
		commands = fmt.Sprintf("%s --initial-cluster-token=%s", commands, token)
	}

	env := []v1.EnvVar{envPodIP}
	labels := map[string]string{
		"app":          "etcd",
		"etcd_node":    name,
		"etcd_cluster": clusterName,
	}

	c := etcdContainer(commands, cs.Version, env)
	// On node reboot, there will be two copies of etcd pod: scheduled and checkpointed one.
	// Checkpointed one will start first. But then the scheduler will detect host port conflict,
	// and set the pod (in APIServer) failed. This further affects etcd service by removing the endpoints.
	// To make scheduling phase succeed, we work around by removing ports in spec.
	// However, the scheduled pod will fail when running on node because resources (e.g. host port) are taken.
	// Thus, we make etcd pod flock first before starting etcd server.
	c.Ports = nil
	c.VolumeMounts = append(c.VolumeMounts, v1.VolumeMount{
		Name:      varLockVolumeName,
		MountPath: varLockDir,
		ReadOnly:  false,
	})
	c.Command = []string{"sh", "-ec", fmt.Sprintf("flock %s -c \"%s\"", etcdLockPath, commands)}
	if cs.Pod != nil {
		c = containerWithRequirements(c, cs.Pod.Resources)
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
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
			Volumes: []v1.Volume{{
				Name: "etcd-data",
				// TODO: configurable mount host path.
				VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: etcdVolumeMountDir}},
			}, {
				Name:         varLockVolumeName,
				VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: varLockDir}},
			}},
		},
	}

	SetEtcdVersion(pod, cs.Version)

	applyPodPolicy(clusterName, pod, cs.Pod)
	// overwrites the antiAffinity setting for self hosted cluster.
	pod = selfHostedPodWithAntiAffinity(pod)

	addOwnerRefToObject(pod.GetObjectMeta(), owner)
	return pod
}

func selfHostedPodWithAntiAffinity(pod *v1.Pod) *v1.Pod {
	// self hosted pods should sit on different nodes even if they are from different cluster.
	ls := &metav1.LabelSelector{MatchLabels: map[string]string{
		"app": "etcd",
	}}
	return podWithAntiAffinity(pod, ls)
}
