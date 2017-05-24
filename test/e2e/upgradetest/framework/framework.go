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
	"os"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	KubeCli  kubernetes.Interface
	S3Cli    *s3.S3
	S3Bucket string
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
		Config:  fc,
		KubeCli: kubecli,
	}
	err = f.setupAWS()
	return f, err
}

func (f *Framework) CreateOperator() error {
	cmd := []string{"/usr/local/bin/etcd-operator", "--analytics=false",
		"--backup-aws-secret=aws", "--backup-aws-config=aws", "--backup-s3-bucket=jenkins-etcd-operator"}
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
						Name:            name,
						Image:           image,
						ImagePullPolicy: v1.PullAlways,
						Command:         cmd,
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
	err := f.KubeCli.AppsV1beta1().Deployments(f.KubeNS).Delete("etcd-operator", k8sutil.CascadeDeleteOptions(0))
	if err != nil {
		return err
	}

	// Wait until the etcd-operator pod is actually gone and not just terminating.
	// In upgrade tests, the next test shouldn't see any etcd operator pod.
	lo := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"name": "etcd-operator"}).String(),
	}
	_, err = e2eutil.WaitPodsDeleted(f.KubeCli, f.KubeNS, 30*time.Second, lo)
	return err
}

func (f *Framework) UpgradeOperator() error {
	uf := func(d *appsv1beta1.Deployment) {
		d.Spec.Template.Spec.Containers[0].Image = f.NewImage
	}
	err := k8sutil.PatchDeployment(f.KubeCli, f.KubeNS, "etcd-operator", uf)
	if err != nil {
		return err
	}

	lo := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"name": "etcd-operator"}).String(),
	}
	_, err = e2eutil.WaitPodsWithImageDeleted(f.KubeCli, f.KubeNS, f.OldImage, 30*time.Second, lo)
	return err
}

func (f *Framework) setupAWS() error {
	if err := os.Setenv("AWS_SHARED_CREDENTIALS_FILE", os.Getenv("AWS_CREDENTIAL")); err != nil {
		return err
	}
	if err := os.Setenv("AWS_CONFIG_FILE", os.Getenv("AWS_CONFIG")); err != nil {
		return err
	}
	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return err
	}
	f.S3Cli = s3.New(sess)
	f.S3Bucket = "jenkins-etcd-operator"
	return nil
}
