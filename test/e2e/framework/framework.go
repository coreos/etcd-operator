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
	"strconv"
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
		CRClient:   client.MustNew(config),
		Namespace:  *ns,
		opImage:    *opImage,
	}
	return Global.setup()
}

func Teardown() error {
	if err := Global.deleteEtcdOperator(); err != nil {
		return err
	}
	err := Global.KubeClient.CoreV1().Pods(Global.Namespace).Delete(etcdBackupOperatorName, metav1.NewDeleteOptions(1))
	if err != nil {
		return fmt.Errorf("failed to delete etcd backup operator: %v", err)
	}
	err = Global.KubeClient.CoreV1().Pods(Global.Namespace).Delete(etcdRestoreOperatorName, metav1.NewDeleteOptions(1))
	if err != nil {
		return fmt.Errorf("failed to delete etcd restore operator pod: %v", err)
	}
	err = Global.KubeClient.CoreV1().Services(Global.Namespace).Delete(etcdRestoreOperatorServiceName, metav1.NewDeleteOptions(1))
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete etcd restore operator service: %v", err)
	}
	// TODO: check all deleted and wait
	Global = nil
	logrus.Info("e2e teardown successfully")
	return nil
}

func (f *Framework) setup() error {
	if err := f.SetupEtcdOperator(); err != nil {
		return fmt.Errorf("failed to setup etcd operator: %v", err)
	}
	logrus.Info("etcd operator created successfully")
	err := f.SetupEtcdBackupOperator()
	if err != nil {
		return fmt.Errorf("failed to create etcd backup operator: %v", err)
	}
	logrus.Info("etcd backup operator created successfully")

	err = f.SetupEtcdRestoreOperatorAndService()
	if err != nil {
		return err
	}
	logrus.Info("etcd restore operator pod and service created successfully")

	logrus.Info("e2e setup successfully")
	return nil
}

func (f *Framework) SetupEtcdOperator() error {
	// TODO: unify this and the yaml file in example/
	cmd := []string{"/usr/local/bin/etcd-operator"}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "etcd-operator",
			Labels: map[string]string{"name": "etcd-operator"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            "etcd-operator",
					Image:           f.opImage,
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
				},
			},
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
	err := f.deleteEtcdOperator()
	if err != nil {
		return err
	}
	// On k8s 1.6.1, grace period isn't accurate. It took ~10s for operator pod to completely disappear.
	// We work around by increasing the wait time. Revisit this later.
	err = retryutil.Retry(5*time.Second, 6, func() (bool, error) {
		_, err := f.KubeClient.CoreV1().Pods(f.Namespace).Get("etcd-operator", metav1.GetOptions{})
		if err == nil {
			return false, nil
		}
		if k8sutil.IsKubernetesResourceNotFoundError(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("fail to wait etcd operator pod gone from API: %v", err)
	}
	return nil
}

// SetupEtcdBackupOperator creates a etcd backup operator pod with name as "etcd-backup-operator".
func (f *Framework) SetupEtcdBackupOperator() error {
	cmd := []string{"/usr/local/bin/etcd-backup-operator"}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   etcdBackupOperatorName,
			Labels: map[string]string{"name": etcdBackupOperatorName},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            etcdBackupOperatorName,
					Image:           f.opImage,
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
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
		},
	}

	p, err := k8sutil.CreateAndWaitPod(f.KubeClient, f.Namespace, pod, 60*time.Second)
	if err != nil {
		describePod(f.Namespace, etcdBackupOperatorName)
		return err
	}
	logrus.Infof("etcd backup operator pod is running on node (%s)", p.Spec.NodeName)
	return nil
}

// SetupEtcdRestoreOperatorAndService creates an etcd restore operator pod and the restore operator service.
func (f *Framework) SetupEtcdRestoreOperatorAndService() error {
	cmd := []string{"/usr/local/bin/etcd-restore-operator"}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   etcdRestoreOperatorName,
			Labels: map[string]string{"name": etcdRestoreOperatorName},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            etcdRestoreOperatorName,
					Image:           f.opImage,
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
						{
							Name:  constants.EnvRestoreOperatorServiceName,
							Value: etcdRestoreOperatorServiceName + ":" + strconv.Itoa(etcdRestoreServicePort),
						},
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
		},
	}

	p, err := k8sutil.CreateAndWaitPod(f.KubeClient, f.Namespace, pod, 60*time.Second)
	if err != nil {
		describePod(f.Namespace, etcdRestoreOperatorName)
		return fmt.Errorf("create restore-operator pod failed: %v", err)
	}
	logrus.Infof("restore-operator pod is running on node (%s)", p.Spec.NodeName)

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   etcdRestoreOperatorServiceName,
			Labels: map[string]string{"name": etcdRestoreOperatorServiceName},
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"name": etcdRestoreOperatorName},
			Ports: []v1.ServicePort{{
				Protocol: v1.ProtocolTCP,
				Port:     etcdRestoreServicePort,
			}},
		},
	}
	_, err = f.KubeClient.CoreV1().Services(f.Namespace).Create(svc)
	if err != nil {
		return fmt.Errorf("create restore-operator service failed: %v", err)
	}
	return nil
}

func (f *Framework) deleteEtcdOperator() error {
	return f.KubeClient.CoreV1().Pods(f.Namespace).Delete("etcd-operator", metav1.NewDeleteOptions(1))
}
