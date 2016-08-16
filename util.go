package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/kubernetes/pkg/api"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/unversioned"

	"k8s.io/kubernetes/pkg/util/intstr"
)

func createEtcdService(kclient *unversioned.Client, etcdName, clusterName string) error {
	svc := makeEtcdService(etcdName, clusterName)
	if _, err := kclient.Services("default").Create(svc); err != nil {
		return err
	}
	return nil
}

// todo: use a struct to replace the huge arg list.
func createEtcdPod(kclient *unversioned.Client, etcdName, clusterName string, initialCluster []string, state string, antiAffinity bool) error {
	pod := makeEtcdPod(etcdName, clusterName, initialCluster, state, antiAffinity)
	if _, err := kclient.Pods("default").Create(pod); err != nil {
		return err
	}
	return nil
}

func makeClientAddr(name string) string {
	return fmt.Sprintf("http://%s:2379", name)
}

func makeEtcdPeerAddr(etcdName string) string {
	return fmt.Sprintf("http://%s:2380", etcdName)
}

func makeEtcdService(etcdName, clusterName string) *api.Service {
	labels := map[string]string{
		"etcd_node":    etcdName,
		"etcd_cluster": clusterName,
	}
	svc := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:   etcdName,
			Labels: labels,
		},
		Spec: api.ServiceSpec{
			Ports: []api.ServicePort{
				{
					Name:       "server",
					Port:       2380,
					TargetPort: intstr.FromInt(2380),
					Protocol:   api.ProtocolTCP,
				},
				{
					Name:       "client",
					Port:       2379,
					TargetPort: intstr.FromInt(2379),
					Protocol:   api.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
	return svc
}

// todo: use a struct to replace the huge arg list.
func makeEtcdPod(etcdName, clusterName string, initialCluster []string, state string, antiAffinity bool) *api.Pod {
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: etcdName,
			Labels: map[string]string{
				"app":          "etcd",
				"etcd_node":    etcdName,
				"etcd_cluster": clusterName,
			},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Command: []string{
						"/usr/local/bin/etcd",
						"--name",
						etcdName,
						"--initial-advertise-peer-urls",
						makeEtcdPeerAddr(etcdName),
						"--listen-peer-urls",
						"http://0.0.0.0:2380",
						"--listen-client-urls",
						"http://0.0.0.0:2379",
						"--advertise-client-urls",
						makeClientAddr(etcdName),
						"--initial-cluster",
						strings.Join(initialCluster, ","),
						"--initial-cluster-state",
						state,
					},
					Name:  etcdName,
					Image: "gcr.io/coreos-k8s-scale-testing/etcd-amd64:3.0.4",
					Ports: []api.ContainerPort{
						{
							Name:          "server",
							ContainerPort: int32(2380),
							Protocol:      api.ProtocolTCP,
						},
					},
				},
			},
			RestartPolicy: api.RestartPolicyNever,
		},
	}

	if !antiAffinity {
		return pod
	}

	// set pod anti-affinity with the pods that belongs to the same etcd cluster
	affinity := api.Affinity{
		PodAntiAffinity: &api.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []api.PodAffinityTerm{
				api.PodAffinityTerm{
					LabelSelector: &unversionedAPI.LabelSelector{
						MatchLabels: map[string]string{
							"etcd_cluster": clusterName,
						},
					},
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
