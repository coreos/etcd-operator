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
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/coreos/etcd-operator/pkg/client"
	"github.com/coreos/etcd-operator/pkg/generated/clientset/versioned"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/probe"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	v1beta1storage "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var Global *Framework

const etcdBackupOperator = "etcd-backup-operator"

type Framework struct {
	opImage          string
	KubeClient       kubernetes.Interface
	CRClient         versioned.Interface
	Namespace        string
	S3Cli            *s3.S3
	S3Bucket         string
	StorageClassName string
	Provisioner      string
}

// Setup setups a test framework and points "Global" to it.
func Setup() error {
	kubeconfig := flag.String("kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
	opImage := flag.String("operator-image", "", "operator image, e.g. gcr.io/coreos-k8s-scale-testing/etcd-operator")
	pvProvisioner := flag.String("pv-provisioner", "kubernetes.io/gce-pd", "persistent volume provisioner type: the default is kubernetes.io/gce-pd. This should be set according to where the tests are running")
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
		KubeClient:       cli,
		CRClient:         client.MustNew(config),
		Namespace:        *ns,
		opImage:          *opImage,
		S3Bucket:         os.Getenv("TEST_S3_BUCKET"),
		StorageClassName: "e2e-" + path.Base(*pvProvisioner),
		Provisioner:      *pvProvisioner,
	}
	return Global.setup()
}

func Teardown() error {
	if err := Global.deleteEtcdOperator(); err != nil {
		return err
	}
	if err := Global.DeleteEtcdBackupOperator(); err != nil {
		return fmt.Errorf("failed to delete etcd backup operator: %v", err)
	}
	// TODO: check all deleted and wait
	Global = nil
	logrus.Info("e2e teardown successfully")
	return nil
}

func (f *Framework) setup() error {
	if err := f.setupStorageClass(); err != nil {
		return fmt.Errorf("failed to setup storageclass(%v): %v", f.StorageClassName, err)
	}
	if err := f.SetupEtcdOperator(); err != nil {
		return fmt.Errorf("failed to setup etcd operator: %v", err)
	}
	logrus.Info("etcd operator created successfully")
	if os.Getenv("AWS_TEST_ENABLED") == "true" {
		if err := f.setupAWS(); err != nil {
			return fmt.Errorf("fail to setup aws: %v", err)
		}
	}
	if err := f.SetupEtcdBackupOperator(); err != nil {
		return fmt.Errorf("failed to create etcd backup operator: %v", err)
	}
	logrus.Info("etcd backup operator created successfully")

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
		// assuming `kubectl` installed on $PATH
		cmd := exec.Command("kubectl", "-n", f.Namespace, "describe", "pod", "etcd-operator")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Run() // Just ignore the error...
		logrus.Info("describing etcd-operator pod:", out.String())
		return err
	}
	logrus.Infof("etcd operator pod is running on node (%s)", p.Spec.NodeName)

	return e2eutil.WaitUntilOperatorReady(f.KubeClient, f.Namespace, "etcd-operator")
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

// SetupEtcdBackupOperator creates a etcd backup operator deployment with name as "etcd-backup-operator",
// and it waits until the operator pod is ready.
func (f *Framework) SetupEtcdBackupOperator() error {
	cmd := []string{"/usr/local/bin/etcd-backup-operator"}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   etcdBackupOperator,
			Labels: map[string]string{"name": etcdBackupOperator},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            etcdBackupOperator,
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
		return err
	}
	logrus.Infof("etcd backup operator pod is running on node (%s)", p.Spec.NodeName)

	return e2eutil.WaitUntilOperatorReady(f.KubeClient, f.Namespace, etcdBackupOperator)
}

// DeleteEtcdBackupOperator deletes an etcd backup operator with the name as "etcd-backup-operator".
func (f *Framework) DeleteEtcdBackupOperator() error {
	return f.KubeClient.CoreV1().Pods(f.Namespace).Delete(etcdBackupOperator, metav1.NewDeleteOptions(1))
}

func (f *Framework) deleteEtcdOperator() error {
	return f.KubeClient.CoreV1().Pods(f.Namespace).Delete("etcd-operator", metav1.NewDeleteOptions(1))
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
	return nil
}

func (f *Framework) setupStorageClass() error {
	class := &v1beta1storage.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: f.StorageClassName,
		},
		Provisioner: f.Provisioner,
	}
	_, err := f.KubeClient.StorageV1beta1().StorageClasses().Create(class)
	if err != nil && !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
		return fmt.Errorf("fail to create storage class: %v", err)
	}
	return nil
}
