// Copyright 2017 The etcd-operator Authors
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

package framework

import (
	"fmt"

	"github.com/coreos/etcd-operator/pkg/client"
	"github.com/coreos/etcd-operator/pkg/generated/clientset/versioned"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/probe"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"

	appsv1beta1 "k8s.io/api/apps/v1beta1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Config struct {
	// program flags
	KubeConfig string
	KubeNS     string
	OldImage   string
	NewImage   string
}

type Framework struct {
	Config
	// global var
	KubeCli  kubernetes.Interface
	CRClient versioned.Interface
}

func New(fc Config) (*Framework, error) {
	kc, err := clientcmd.BuildConfigFromFlags("", fc.KubeConfig)
	if err != nil {
		return nil, err
	}
	kubecli, err := kubernetes.NewForConfig(kc)
	if err != nil {
		return nil, err
	}

	f := &Framework{
		Config:   fc,
		KubeCli:  kubecli,
		CRClient: client.MustNew(kc),
	}
	return f, nil
}

func (f *Framework) CreateOperator(name string) error {
	cmd := []string{"/usr/local/bin/etcd-operator"}
	image := f.OldImage
	d := &appsv1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.KubeNS,
		},
		Spec: appsv1beta1.DeploymentSpec{
			Strategy: appsv1beta1.DeploymentStrategy{
				Type: appsv1beta1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			},
			Selector: &metav1.LabelSelector{MatchLabels: operatorLabelSelector(name)},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: operatorLabelSelector(name),
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name:            name,
						Image:           image,
						ImagePullPolicy: v1.PullAlways,
						Command:         cmd,
						Env: []v1.EnvVar{
							{
								Name:      constants.EnvOperatorPodNamespace,
								ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
							},
							{
								Name:      constants.EnvOperatorPodName,
								ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"}},
							},
						},
						ReadinessProbe: &v1.Probe{
							Handler: v1.Handler{
								HTTPGet: &v1.HTTPGetAction{
									Path: probe.HTTPReadyzEndpoint,
									Port: intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
								},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       3,
							FailureThreshold:    3,
						},
					}},
				},
			},
		},
	}
	_, err := f.KubeCli.AppsV1beta1().Deployments(f.KubeNS).Create(d)
	if err != nil {
		return fmt.Errorf("failed to create deployment: %v", err)
	}
	return nil
}

func (f *Framework) DeleteOperator(name string) error {
	err := f.KubeCli.AppsV1beta1().Deployments(f.KubeNS).Delete(name, k8sutil.CascadeDeleteOptions(0))
	if err != nil {
		return err
	}

	// Wait until the etcd-operator pod is actually gone and not just terminating.
	// In upgrade tests, the next test shouldn't see any etcd operator pod.
	lo := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(operatorLabelSelector(name)).String(),
	}
	_, err = e2eutil.WaitPodsDeletedCompletely(f.KubeCli, f.KubeNS, 3, lo)
	if err != nil {
		return err
	}
	// The deleted operator will not actively release the Endpoints lock causing a non-leader candidate to timeout for the lease duration: 15s
	// Deleting the Endpoints resource simulates the leader actively releasing the lock so that the next candidate avoids the timeout.
	// TODO: change this if we change to use another kind of lock, e.g. configmap.
	return f.KubeCli.CoreV1().Endpoints(f.KubeNS).Delete("etcd-operator", metav1.NewDeleteOptions(0))
}

func (f *Framework) UpgradeOperator(name string) error {
	uf := func(d *appsv1beta1.Deployment) {
		d.Spec.Template.Spec.Containers[0].Image = f.NewImage
	}
	err := k8sutil.PatchDeployment(f.KubeCli, f.KubeNS, name, uf)
	if err != nil {
		return err
	}

	lo := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(operatorLabelSelector(name)).String(),
	}
	_, err = e2eutil.WaitPodsWithImageDeleted(f.KubeCli, f.KubeNS, f.OldImage, 3, lo)
	if err != nil {
		return fmt.Errorf("failed to wait for pod with old image to get deleted: %v", err)
	}
	err = e2eutil.WaitUntilOperatorReady(f.KubeCli, f.KubeNS, name)
	return err
}

func operatorLabelSelector(name string) map[string]string {
	return e2eutil.NameLabelSelector(name)
}
