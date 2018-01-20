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

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func NewSelfHostedEtcdPod(m *etcdutil.Member, initialCluster, endpoints []string, clusterName, state, token string, cs api.ClusterSpec, owner metav1.OwnerReference) *v1.Pod {
	hostDataDir := selfHostedDataDir(m.Namespace, m.Name)
	commands := fmt.Sprintf("/usr/local/bin/etcd --data-dir=%s --name=%s --initial-advertise-peer-urls=%s "+
		"--listen-peer-urls=%s --listen-client-urls=%s --advertise-client-urls=%s "+
		"--initial-cluster=%s --initial-cluster-state=%s",
		hostDataDir, m.Name, m.PeerURL(), m.ListenPeerURL(), m.ListenClientURL(), m.ClientURL(), strings.Join(initialCluster, ","), state)
	if m.SecurePeer {
		commands += fmt.Sprintf(" --peer-client-cert-auth=true --peer-trusted-ca-file=%[1]s/peer-ca.crt --peer-cert-file=%[1]s/peer.crt --peer-key-file=%[1]s/peer.key", peerTLSDir)
	}
	if m.SecureClient {
		commands += fmt.Sprintf(" --client-cert-auth=true --trusted-ca-file=%[1]s/server-ca.crt --cert-file=%[1]s/server.crt --key-file=%[1]s/server.key", serverTLSDir)
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

	// When scaling from 1 -> 2 members, if DNS entry is not populated yet, the k8s control plane will go down
	// and the etcd pod will not have any chance to talk to each other again. We need to make sure DNS entry ready.
	// TODO: nslookup should timeout if blocked for a while (10s).
	ft := `
while ( ! nslookup %s )
do
	sleep 3
done
%s
`
	commands = fmt.Sprintf(ft, m.Addr(), commands)
	commands = fmt.Sprintf("%s; %s", appendHostsCommands(), commands)
	commands = fmt.Sprintf("flock %s -c \"%s\"", etcdLockPath, commands)
	c := etcdContainer([]string{"/bin/sh", "-ec", commands}, cs.Repository, cs.Version)
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
			MountPath: serverTLSDir,
			Name:      serverTLSVolume,
		}, v1.VolumeMount{
			MountPath: operatorEtcdTLSDir,
			Name:      operatorEtcdTLSVolume,
		})
		volumes = append(volumes, v1.Volume{Name: serverTLSVolume, VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{SecretName: cs.TLS.Static.Member.ServerSecret},
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
			RestartPolicy:                v1.RestartPolicyAlways,
			Containers:                   []v1.Container{c},
			Volumes:                      volumes,
			HostNetwork:                  true,
			DNSPolicy:                    v1.DNSClusterFirstWithHostNet,
			Hostname:                     m.Name,
			Subdomain:                    clusterName,
			AutomountServiceAccountToken: func(b bool) *bool { return &b }(false),
		},
	}

	SetEtcdVersion(pod, cs.Version)

	applyPodPolicy(clusterName, pod, cs.Pod)
	// overwrites the antiAffinity setting for self hosted cluster.
	applyAntiAffinityOnNodes(pod)
	addOwnerRefToObject(pod.GetObjectMeta(), owner)
	return pod
}

func appendHostsCommands() string {
	etcdHostsFile := path.Join(etcdVolumeMountDir, "etcd-hosts.checkpoint")
	// If hosts checkpoint exists, we will append it to /etc/hosts.
	// TODO: remove this when host alias works for hostNetwork Pod.
	return fmt.Sprintf("([ -f %[1]s ] && (cat %[1]s >> /etc/hosts) || true)", etcdHostsFile)
}

func applyAntiAffinityOnNodes(pod *v1.Pod) {
	// self hosted pods should sit on different nodes even if they are from different cluster.
	ls := &metav1.LabelSelector{MatchLabels: map[string]string{
		"app": "etcd",
	}}

	if pod.Spec.Affinity == nil {
		pod.Spec.Affinity = &v1.Affinity{}
	}

	pod.Spec.Affinity.PodAntiAffinity = &v1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
			{
				LabelSelector: ls,
				TopologyKey:   "kubernetes.io/hostname",
			},
		},
	}
}
