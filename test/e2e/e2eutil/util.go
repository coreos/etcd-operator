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

package e2eutil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

func waitResourcesDeleted(t *testing.T, kubeClient kubernetes.Interface, cl *spec.Cluster) error {
	err := retryutil.Retry(5*time.Second, 5, func() (done bool, err error) {
		list, err := kubeClient.CoreV1().Pods(cl.Metadata.Namespace).List(k8sutil.ClusterListOpt(cl.Metadata.Name))
		if err != nil {
			return false, err
		}
		if len(list.Items) > 0 {
			p := list.Items[0]
			logfWithTimestamp(t, "waiting pod (%s) to be deleted.", p.Name)

			buf := bytes.NewBuffer(nil)
			buf.WriteString("init container status:\n")
			printContainerStatus(buf, p.Status.InitContainerStatuses)
			buf.WriteString("container status:\n")
			printContainerStatus(buf, p.Status.ContainerStatuses)
			t.Logf("pod (%s) status.phase is (%s): %v", p.Name, p.Status.Phase, buf.String())

			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("fail to wait pods deleted: %v", err)
	}

	err = retryutil.Retry(5*time.Second, 5, func() (done bool, err error) {
		list, err := kubeClient.CoreV1().Services(cl.Metadata.Namespace).List(k8sutil.ClusterListOpt(cl.Metadata.Name))
		if err != nil {
			return false, err
		}
		if len(list.Items) > 0 {
			logfWithTimestamp(t, "waiting service (%s) to be deleted", list.Items[0].Name)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("fail to wait services deleted: %v", err)
	}
	return nil
}

func waitBackupDeleted(kubeClient kubernetes.Interface, cl *spec.Cluster, storageCheckerOptions *StorageCheckerOptions) error {
	err := retryutil.Retry(5*time.Second, 5, func() (bool, error) {
		_, err := kubeClient.AppsV1beta1().Deployments(cl.Metadata.Namespace).Get(k8sutil.BackupSidecarName(cl.Metadata.Name), metav1.GetOptions{})
		if err == nil {
			return false, nil
		}
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("failed to wait backup Deployment deleted: %v", err)
	}
	err = retryutil.Retry(5*time.Second, 2, func() (done bool, err error) {
		ls := labels.SelectorFromSet(map[string]string{
			"app":          k8sutil.BackupPodSelectorAppField,
			"etcd_cluster": cl.Metadata.Name,
		}).String()
		pl, err := kubeClient.CoreV1().Pods(cl.Metadata.Namespace).List(metav1.ListOptions{
			LabelSelector: ls,
		})
		if err != nil {
			return false, err
		}
		if len(pl.Items) == 0 {
			return true, nil
		}
		if pl.Items[0].DeletionTimestamp != nil {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait backup pod terminated: %v", err)
	}
	// The rest is to track backup storage, e.g. PV or S3 "dir" deleted.
	// If CleanupBackupsOnClusterDelete=false, we don't delete them and thus don't check them.
	if !cl.Spec.Backup.CleanupBackupsOnClusterDelete {
		return nil
	}
	err = retryutil.Retry(5*time.Second, 5, func() (done bool, err error) {
		switch cl.Spec.Backup.StorageType {
		case spec.BackupStorageTypePersistentVolume, spec.BackupStorageTypeDefault:
			pl, err := kubeClient.CoreV1().PersistentVolumeClaims(cl.Metadata.Namespace).List(k8sutil.ClusterListOpt(cl.Metadata.Name))
			if err != nil {
				return false, err
			}
			if len(pl.Items) > 0 {
				return false, nil
			}
		case spec.BackupStorageTypeS3:
			resp, err := storageCheckerOptions.S3Cli.ListObjects(&s3.ListObjectsInput{
				Bucket: aws.String(storageCheckerOptions.S3Bucket),
				Prefix: aws.String(cl.Metadata.Name + "/"),
			})
			if err != nil {
				return false, err
			}
			if len(resp.Contents) > 0 {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait storage (%s) to be deleted: %v", cl.Spec.Backup.StorageType, err)
	}
	return nil
}

func printContainerStatus(buf *bytes.Buffer, ss []v1.ContainerStatus) {
	for _, s := range ss {
		if s.State.Waiting != nil {
			buf.WriteString(fmt.Sprintf("%s: Waiting: message (%s) reason (%s)\n", s.Name, s.State.Waiting.Message, s.State.Waiting.Reason))
		}
		if s.State.Terminated != nil {
			buf.WriteString(fmt.Sprintf("%s: Terminated: message (%s) reason (%s)\n", s.Name, s.State.Terminated.Message, s.State.Terminated.Reason))
		}
	}
}
