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
	"bytes"
	"flag"
	"fmt"
	"os/exec"
	"time"

	"github.com/coreos/etcd-operator/pkg/client"
	"github.com/coreos/etcd-operator/pkg/generated/clientset/versioned"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/probe"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var Global *Framework

const (
	etcdBackupOperatorName         = "etcd-backup-operator"
	etcdRestoreOperatorName        = "etcd-restore-operator"
	etcdRestoreOperatorServiceName = "etcd-restore-operator"
	etcdRestoreServicePort         = 19999
)

type Framework struct {
	opImage    string
	KubeClient kubernetes.Interface
	CRClient   versioned.Interface
	Namespace  string
}

// Setup setups a test framework and points "Global" to it.
func setup() error {
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
		CRClient:   client.MustNew(config),
		Namespace:  *ns,
		opImage:    *opImage,
	}

	// Skip the etcd-operator deployment setup if the operator image was not specified
	if len(Global.opImage) == 0 {
		return nil
	}

	return Global.setup()
}

func teardown() error {
	// Skip the etcd-operator teardown if the operator image was not specified
	if len(Global.opImage) == 0 {
		return nil
	}

	err := Global.deleteOperatorCompletely("etcd-operator")
	if err != nil {
		return err
	}
	err = Global.KubeClient.CoreV1().Services(Global.Namespace).Delete(etcdRestoreOperatorServiceName, metav1.NewDeleteOptions(1))
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete etcd restore operator service: %v", err)
	}
	Global = nil
	logrus.Info("e2e teardown successfully")
	return nil
}

func (f *Framework) setup() error {
	err := f.SetupEtcdOperator()
	if err != nil {
		return fmt.Errorf("failed to setup etcd operator: %v", err)
	}
	logrus.Info("etcd operator created successfully")

	logrus.Info("e2e setup successfully")
	return nil
}

func (f *Framework) SetupEtcdOperator() error {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "etcd-operator",
			Labels: map[string]string{"name": "etcd-operator"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name:            "etcd-operator",
				Image:           f.opImage,
				ImagePullPolicy: v1.PullAlways,
				Command:         []string{"/usr/local/bin/etcd-operator"},
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
			}, {
				Name:            etcdBackupOperatorName,
				Image:           f.opImage,
				ImagePullPolicy: v1.PullAlways,
				Command:         []string{"/usr/local/bin/etcd-backup-operator"},
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
			}, {
				Name:            etcdRestoreOperatorName,
				Image:           f.opImage,
				ImagePullPolicy: v1.PullAlways,
				Command:         []string{"/usr/local/bin/etcd-restore-operator"},
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
			}},
			RestartPolicy: v1.RestartPolicyNever,
		},
	}

	p, err := k8sutil.CreateAndWaitPod(f.KubeClient, f.Namespace, pod, 60*time.Second)
	if err != nil {
		describePod(f.Namespace, "etcd-operator")
		return err
	}
	logrus.Infof("etcd operator pod is running on node (%s)", p.Spec.NodeName)

	return e2eutil.WaitUntilOperatorReady(f.KubeClient, f.Namespace, "etcd-operator")
}

func describePod(ns, name string) {
	// assuming `kubectl` installed on $PATH
	cmd := exec.Command("kubectl", "-n", ns, "describe", "pod", name)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Run() // Just ignore the error...
	logrus.Infof("describing %s pod: %s", name, out.String())
}

func (f *Framework) DeleteEtcdOperatorCompletely() error {
	return f.deleteOperatorCompletely("etcd-operator")
}

func (f *Framework) deleteOperatorCompletely(name string) error {
	err := f.KubeClient.CoreV1().Pods(f.Namespace).Delete(name, metav1.NewDeleteOptions(1))
	if err != nil {
		return err
	}
	// Grace period isn't exactly accurate. It took ~10s for operator pod to completely disappear.
	// We work around by increasing the wait time. Revisit this later.
	err = retryutil.Retry(5*time.Second, 6, func() (bool, error) {
		_, err := f.KubeClient.CoreV1().Pods(f.Namespace).Get(name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}
		if k8sutil.IsKubernetesResourceNotFoundError(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("fail to wait operator (%s) pod gone from API: %v", name, err)
	}
	return nil
}

// SetupEtcdRestoreOperatorService creates restore operator service that is used by etcd pod to retrieve backup.
func (f *Framework) SetupEtcdRestoreOperatorService() error {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: etcdRestoreOperatorServiceName,
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"name": "etcd-operator"},
			Ports: []v1.ServicePort{{
				Protocol: v1.ProtocolTCP,
				Port:     etcdRestoreServicePort,
			}},
		},
	}
	_, err := f.KubeClient.CoreV1().Services(f.Namespace).Create(svc)
	if err != nil {
		return fmt.Errorf("create restore-operator service failed: %v", err)
	}
	return nil
}
