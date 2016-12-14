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
	"time"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
)

var Global *Framework

type Framework struct {
	KubeClient *unversioned.Client
	MasterHost string
	Namespace  string
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
	cli, err := unversioned.New(config)
	if err != nil {
		return err
	}

	Global = &Framework{
		MasterHost: config.Host,
		KubeClient: cli,
		Namespace:  *ns,
	}
	return Global.setup(*opImage)
}

func Teardown() error {
	// TODO: check all deleted and wait
	if err := Global.KubeClient.Pods(Global.Namespace).Delete("etcd-operator", api.NewDeleteOptions(0)); err != nil {
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
	logrus.Info("e2e setup successfully")
	return nil
}

func (f *Framework) setupEtcdOperator(opImage string) error {
	// TODO: unify this and the yaml file in example/
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name:   "etcd-operator",
			Labels: map[string]string{"name": "etcd-operator"},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Name:  "etcd-operator",
					Image: opImage,
					Command: []string{
						"/bin/sh", "-c",
						"/usr/local/bin/etcd-operator --analytics=false",
					},
					Env: []api.EnvVar{
						{
							Name:      "MY_POD_NAMESPACE",
							ValueFrom: &api.EnvVarSource{FieldRef: &api.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
						},
					},
				},
			},
			RestartPolicy: api.RestartPolicyNever,
		},
	}

	_, err := k8sutil.CreateAndWaitPod(f.KubeClient, f.Namespace, pod, 60*time.Second)
	if err != nil {
		return err
	}
	err = k8sutil.WaitEtcdTPRReady(f.KubeClient.Client, 5*time.Second, 60*time.Second, f.MasterHost, f.Namespace)
	if err != nil {
		return err
	}

	logrus.Info("etcd operator created successfully")
	return nil
}
