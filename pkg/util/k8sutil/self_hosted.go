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
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	shouldCheckpointAnnotation = "checkpointer.alpha.coreos.com/checkpoint" // = "true"
	varLockVolumeName          = "var-lock"
	varLockDir                 = "/var/lock"
	etcdLockPath               = "/var/lock/etcd.lock"
)

func selfHostedDataDir(ns, name string) string {
	return path.Join(etcdVolumeMountDir, ns+"-"+name)
}

func NewSelfHostedEtcdPod(m *etcdutil.Member, initialCluster, endpoints []string, clusterName, state, token string, cs spec.ClusterSpec, owner metav1.OwnerReference) *v1.Pod {
	hostDataDir := selfHostedDataDir(m.Namespace, m.Name)
	commands := fmt.Sprintf("/usr/local/bin/etcd --data-dir=%s --name=%s --initial-advertise-peer-urls=%s "+
		"--listen-peer-urls=%s --listen-client-urls=%s --advertise-client-urls=%s "+
		"--initial-cluster=%s --initial-cluster-state=%s",
		hostDataDir, m.Name, m.PeerURL(), m.ListenPeerURL(), m.ListenClientURL(), m.ClientAddr(), strings.Join(initialCluster, ","), state)
	if m.SecurePeer {
		commands += fmt.Sprintf(" --peer-client-cert-auth=true --peer-trusted-ca-file=%[1]s/peer-ca-crt.pem --peer-cert-file=%[1]s/peer-crt.pem --peer-key-file=%[1]s/peer-key.pem", peerTLSDir)
	}
	if m.SecureClient {
		commands += fmt.Sprintf(" --client-cert-auth=true --trusted-ca-file=%[1]s/client-ca-crt.pem --cert-file=%[1]s/client-crt.pem --key-file=%[1]s/client-key.pem", clientTLSDir)
	}
	if state == "new" {
		commands += fmt.Sprintf(" --initial-cluster-token=%s", token)
	}

	labels := map[string]string{
		"app":          "etcd",
		"etcd_node":    m.Name,
		"etcd_cluster": clusterName,
	}

	if len(endpoints) > 0 {
		addMemberCmd := fmt.Sprintf("ETCDCTL_API=3 etcdctl --endpoints=%s member add %s --peer-urls=%s", strings.Join(endpoints, ","), m.Name, m.PeerURL())
		if m.SecureClient {
			addMemberCmd += fmt.Sprintf(" --cert=%[1]s/%[2]s --key=%[1]s/%[3]s --cacert=%[1]s/%[4]s",
				operatorEtcdTLSDir, etcdutil.CliCertFile, etcdutil.CliKeyFile, etcdutil.CliCAFile)
		}
		commands = fmt.Sprintf("([ -d %s ] || %s); %s", hostDataDir, addMemberCmd, commands)
	}

	c := etcdContainer(fmt.Sprintf("sleep 5; %s", commands), cs.Version)
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

	volumes := []v1.Volume{{
		Name: etcdVolumeName,
		// TODO: configurable mount host path.
		VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: etcdVolumeMountDir}},
	}, {
		Name:         varLockVolumeName,
		VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: varLockDir}},
	}}
	if m.SecurePeer {
		c.VolumeMounts = append(c.VolumeMounts, v1.VolumeMount{
			MountPath: peerTLSDir,
			Name:      peerTLSVolume,
		})
		volumes = append(volumes, v1.Volume{Name: peerTLSVolume, VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{SecretName: cs.TLS.Static.Member.PeerSecret},
		}})
	}
	if m.SecureClient {
		c.VolumeMounts = append(c.VolumeMounts, v1.VolumeMount{
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
			Name:   m.Name,
			Labels: labels,
			Annotations: map[string]string{
				shouldCheckpointAnnotation: "true",
			},
		},
		Spec: v1.PodSpec{
			// Self-hosted etcd pod need to endure node restart.
			// If we set it to Never, the pod won't restart. If etcd won't come up, nor does other k8s API components.
			RestartPolicy: v1.RestartPolicyAlways,
			Containers:    []v1.Container{c},
			Volumes:       volumes,
			HostNetwork:   true,
			DNSPolicy:     v1.DNSClusterFirstWithHostNet,
			Hostname:      m.Name,
			Subdomain:     clusterName,
		},
	}

	SetEtcdVersion(pod, cs.Version)

	applyPodPolicy(clusterName, pod, cs.Pod)
	// overwrites the antiAffinity setting for self hosted cluster.
	pod = selfHostedPodWithAntiAffinity(pod)
	applyAppendHostsInitContainer(pod)
	addOwnerRefToObject(pod.GetObjectMeta(), owner)
	return pod
}

func applyAppendHostsInitContainer(p *v1.Pod) {
	etcdHostsFile := path.Join(etcdVolumeMountDir, "etcd-hosts.checkpoint")
	containerSpec := []v1.Container{
		{
			Name:  "append-hosts",
			Image: "busybox",
			Command: []string{
				// Init container would be re-executed on restart. We are taking the datadir as a signal of restart.
				// If restart happens and hosts checkpoint exists, we will append it to /etc/hosts.
				"/bin/sh", "-c",
				fmt.Sprintf("[ -f %[1]s ] && (cat %[1]s >> /etc/hosts) || true", etcdHostsFile),
			},
			VolumeMounts: []v1.VolumeMount{
				{Name: etcdVolumeName, MountPath: etcdVolumeMountDir},
			},
		},
	}

	p.Spec.InitContainers = containerSpec
}

func selfHostedPodWithAntiAffinity(pod *v1.Pod) *v1.Pod {
	// self hosted pods should sit on different nodes even if they are from different cluster.
	ls := &metav1.LabelSelector{MatchLabels: map[string]string{
		"app": "etcd",
	}}
	return podWithAntiAffinity(pod, ls)
}
