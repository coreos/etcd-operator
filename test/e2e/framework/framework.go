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

package framework

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/tools/clientcmd"
)

var Global *Framework

type Framework struct {
	KubeClient kubernetes.Interface
	Namespace  string
	S3Cli      *s3.S3
	S3Bucket   string
}

// Setup setups a test framework and points "Global" to it.
func Setup() error {
	kubeconfig := flag.String("kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
	opImage := flag.String("operator-image", "", "operator image, e.g. gcr.io/coreos-k8s-scale-testing/etcd-operator")
	ns := flag.String("namespace", "default", "e2e test namespace")
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return err
	}
	cli, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	Global = &Framework{
		KubeClient: cli,
		Namespace:  *ns,
	}
	return Global.setup(*opImage)
}

func Teardown() error {
	// TODO: check all deleted and wait
	if err := Global.KubeClient.Core().Pods(Global.Namespace).Delete("etcd-operator", api.NewDeleteOptions(10)); err != nil {
		return err
	}
	Global = nil
	logrus.Info("e2e teardown successfully")
	return nil
}

func (f *Framework) setup(opImage string) error {
	if err := f.setupEtcdOperator(opImage); err != nil {
		logrus.Errorf("fail to setup etcd operator: %v", err)
		return err
	}
	if os.Getenv("AWS_TEST_ENABLED") == "true" {
		if err := f.setupAWS(); err != nil {
			return fmt.Errorf("fail to setup aws: %v", err)
		}
	}
	logrus.Info("e2e setup successfully")
	return nil
}

func (f *Framework) setupEtcdOperator(opImage string) error {
	// TODO: unify this and the yaml file in example/
	cmd := "/usr/local/bin/etcd-operator --analytics=false"
	if os.Getenv("AWS_TEST_ENABLED") == "true" {
		cmd += " --backup-aws-secret=aws --backup-aws-config=aws --backup-s3-bucket=jenkins-etcd-operator"
	}
	pod := &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:   "etcd-operator",
			Labels: map[string]string{"name": "etcd-operator"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "etcd-operator",
					Image: opImage,
					Command: []string{
						"/bin/sh", "-c", cmd,
					},
					Env: []v1.EnvVar{
						{
							Name:      "MY_POD_NAMESPACE",
							ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
						},
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
		},
	}

	_, err := k8sutil.CreateAndWaitPod(f.KubeClient, f.Namespace, pod, 60*time.Second)
	if err != nil {
		return err
	}
	err = k8sutil.WaitEtcdTPRReady(f.KubeClient.Core().GetRESTClient(), 5*time.Second, 60*time.Second, f.Namespace)
	if err != nil {
		return err
	}

	logrus.Info("etcd operator created successfully")
	return nil
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
