// Copyright 2018 The etcd-operator Authors
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

package main

import (
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// createServiceForMyself gets restore-operator pod labels, strip away "pod-template-hash",
// and then use it as selector to create a service for current restore-operator.
func createServiceForMyself(kubecli kubernetes.Interface, name, namespace string) error {
	pod, err := kubecli.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return errors.WithStack(err)
	}
	// strip away replicaset-specific label added by deployment.
	delete(pod.Labels, "pod-template-hash")
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceNameForMyself,
			Namespace: namespace,
			Labels:    pod.Labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port:       int32(servicePortForMyself),
				TargetPort: intstr.FromInt(servicePortForMyself),
				Protocol:   v1.ProtocolTCP,
			}},
			Selector: pod.Labels,
		},
	}
	_, err = kubecli.CoreV1().Services(namespace).Create(svc)
	if err != nil && !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
		return errors.WithStack(err)
	}
	return nil
}
