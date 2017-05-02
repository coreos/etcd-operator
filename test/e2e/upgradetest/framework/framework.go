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
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	appsv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
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
	KubeCli kubernetes.Interface
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

	return &Framework{
		Config:  fc,
		KubeCli: kubecli,
	}, nil
}

func (f *Framework) CreateOperator() error {
	cmd := []string{"/usr/local/bin/etcd-operator", "--analytics=false"}
	name := "etcd-operator"
	image := f.OldImage
	selector := map[string]string{"name": "etcd-operator"}
	d := &appsv1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.KubeNS,
		},
		Spec: appsv1beta1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selector,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name:    "etcd-operator",
						Image:   image,
						Command: cmd,
						Env: []v1.EnvVar{
							{
								Name:      "MY_POD_NAMESPACE",
								ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
							},
							{
								Name:      "MY_POD_NAME",
								ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"}},
							},
						},
					}},
				},
			},
		},
	}
	_, err := f.KubeCli.AppsV1beta1().Deployments(f.KubeNS).Create(d)
	return err
}

func (f *Framework) DeleteOperator() error {
	foreground := metav1.DeletePropagationForeground
	return f.KubeCli.AppsV1beta1().Deployments(f.KubeNS).Delete("etcd-operator", &metav1.DeleteOptions{
		PropagationPolicy: &foreground,
	})
}

func (f *Framework) UpgradeOperator() error {
	image := f.NewImage
	d, err := f.KubeCli.AppsV1beta1().Deployments(f.KubeNS).Get("etcd-operator", metav1.GetOptions{})
	if err != nil {
		return err
	}
	cd := k8sutil.CloneDeployment(d)
	cd.Spec.Template.Spec.Containers[0].Image = image
	patchData, err := k8sutil.CreatePatch(d, cd, appsv1beta1.Deployment{})
	if err != nil {
		return err
	}
	_, err = f.KubeCli.AppsV1beta1().Deployments(f.KubeNS).Patch(d.Name, types.StrategicMergePatchType, patchData)
	return err
}
