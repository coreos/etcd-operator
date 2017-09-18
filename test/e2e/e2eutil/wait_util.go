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
	"context"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta1"
	backups3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/client"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

var retryInterval = 10 * time.Second

type acceptFunc func(*api.EtcdCluster) bool
type filterFunc func(*v1.Pod) bool

func CalculateRestoreWaitTime(needDataClone bool) int {
	waitTime := 24
	if needDataClone {
		// Take additional time to clone the data.
		waitTime += 6
	}
	return waitTime
}

func WaitUntilPodSizeReached(t *testing.T, kubeClient kubernetes.Interface, size, retries int, cl *api.EtcdCluster) ([]string, error) {
	var names []string
	err := retryutil.Retry(retryInterval, retries, func() (done bool, err error) {
		podList, err := kubeClient.Core().Pods(cl.Namespace).List(k8sutil.ClusterListOpt(cl.Name))
		if err != nil {
			return false, err
		}
		names = nil
		var nodeNames []string
		for i := range podList.Items {
			pod := &podList.Items[i]
			if pod.Status.Phase != v1.PodRunning {
				continue
			}
			names = append(names, pod.Name)
			nodeNames = append(nodeNames, pod.Spec.NodeName)
		}
		LogfWithTimestamp(t, "waiting size (%d), etcd pods: names (%v), nodes (%v)", size, names, nodeNames)
		if len(names) != size {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

func WaitUntilSizeReached(t *testing.T, crClient client.EtcdClusterCR, size, retries int, cl *api.EtcdCluster) ([]string, error) {
	return waitSizeReachedWithAccept(t, crClient, size, retries, cl)
}

func WaitSizeAndVersionReached(t *testing.T, kubeClient kubernetes.Interface, version string, size, retries int, cl *api.EtcdCluster) error {
	return retryutil.Retry(retryInterval, retries, func() (done bool, err error) {
		var names []string
		podList, err := kubeClient.Core().Pods(cl.Namespace).List(k8sutil.ClusterListOpt(cl.Name))
		if err != nil {
			return false, err
		}
		names = nil
		var nodeNames []string
		for i := range podList.Items {
			pod := &podList.Items[i]
			if pod.Status.Phase != v1.PodRunning {
				continue
			}

			containerVersion := getVersionFromImage(pod.Status.ContainerStatuses[0].Image)
			if containerVersion != version {
				LogfWithTimestamp(t, "pod(%v): expected version(%v) current version(%v)", pod.Name, version, containerVersion)
				continue
			}

			names = append(names, pod.Name)
			nodeNames = append(nodeNames, pod.Spec.NodeName)
		}
		LogfWithTimestamp(t, "waiting size (%d), etcd pods: names (%v), nodes (%v)", size, names, nodeNames)
		if len(names) != size {
			return false, nil
		}
		return true, nil
	})
}

func getVersionFromImage(image string) string {
	return strings.Split(image, ":v")[1]
}

func waitSizeReachedWithAccept(t *testing.T, crClient client.EtcdClusterCR, size, retries int, cl *api.EtcdCluster, accepts ...acceptFunc) ([]string, error) {
	var names []string
	err := retryutil.Retry(retryInterval, retries, func() (done bool, err error) {
		currCluster, err := crClient.Get(context.TODO(), cl.Namespace, cl.Name)
		if err != nil {
			return false, err
		}

		for _, accept := range accepts {
			if !accept(currCluster) {
				return false, nil
			}
		}

		names = currCluster.Status.Members.Ready
		LogfWithTimestamp(t, "waiting size (%d), healthy etcd members: names (%v)", size, names)
		if len(names) != size {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

func WaitUntilMembersWithNamesDeleted(t *testing.T, crClient client.EtcdClusterCR, retries int, cl *api.EtcdCluster, targetNames ...string) ([]string, error) {
	var remaining []string
	err := retryutil.Retry(retryInterval, retries, func() (done bool, err error) {
		currCluster, err := crClient.Get(context.TODO(), cl.Namespace, cl.Name)
		if err != nil {
			return false, err
		}

		readyMembers := currCluster.Status.Members.Ready
		remaining = nil
		for _, name := range targetNames {
			if presentIn(name, readyMembers) {
				remaining = append(remaining, name)
			}
		}

		LogfWithTimestamp(t, "waiting on members (%v) to be deleted", remaining)
		if len(remaining) != 0 {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return remaining, nil
}

func presentIn(a string, list []string) bool {
	for _, l := range list {
		if a == l {
			return true
		}
	}
	return false
}

func WaitBackupPodUp(t *testing.T, kubecli kubernetes.Interface, ns, clusterName string, retries int) error {
	ls := labels.SelectorFromSet(k8sutil.BackupSidecarLabels(clusterName))
	return retryutil.Retry(retryInterval, retries, func() (done bool, err error) {
		podList, err := kubecli.CoreV1().Pods(ns).List(metav1.ListOptions{
			LabelSelector: ls.String(),
		})
		if err != nil {
			return false, err
		}
		for i := range podList.Items {
			if podList.Items[i].Status.Phase == v1.PodRunning {
				LogfWithTimestamp(t, "backup pod (%s) is running", podList.Items[i].Name)
				return true, nil
			}
		}
		return false, nil
	})
}

func waitResourcesDeleted(t *testing.T, kubeClient kubernetes.Interface, cl *api.EtcdCluster) error {
	undeletedPods, err := WaitPodsDeleted(kubeClient, cl.Namespace, 3, k8sutil.ClusterListOpt(cl.Name))
	if err != nil {
		if retryutil.IsRetryFailure(err) && len(undeletedPods) > 0 {
			p := undeletedPods[0]
			LogfWithTimestamp(t, "waiting pod (%s) to be deleted.", p.Name)

			buf := bytes.NewBuffer(nil)
			buf.WriteString("init container status:\n")
			printContainerStatus(buf, p.Status.InitContainerStatuses)
			buf.WriteString("container status:\n")
			printContainerStatus(buf, p.Status.ContainerStatuses)
			t.Logf("pod (%s) status.phase is (%s): %v", p.Name, p.Status.Phase, buf.String())
		}

		return fmt.Errorf("fail to wait pods deleted: %v", err)
	}

	err = retryutil.Retry(retryInterval, 3, func() (done bool, err error) {
		list, err := kubeClient.CoreV1().Services(cl.Namespace).List(k8sutil.ClusterListOpt(cl.Name))
		if err != nil {
			return false, err
		}
		if len(list.Items) > 0 {
			LogfWithTimestamp(t, "waiting service (%s) to be deleted", list.Items[0].Name)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("fail to wait services deleted: %v", err)
	}
	return nil
}

func WaitBackupDeleted(kubeClient kubernetes.Interface, cl *api.EtcdCluster, checkerOpt StorageCheckerOptions) error {
	retries := 3
	if checkerOpt.DeletedFromAPI {
		// Currently waiting deployment to be gone from API takes a lot of time.
		// TODO: revisit this when we use "background propagate" deletion policy.
		retries = 30
	}
	err := retryutil.Retry(retryInterval, retries, func() (bool, error) {
		d, err := kubeClient.AppsV1beta1().Deployments(cl.Namespace).Get(k8sutil.BackupSidecarName(cl.Name), metav1.GetOptions{})
		// If we don't need to wait deployment to be completely gone, we can say it is deleted
		// as long as DeletionTimestamp is not nil. Otherwise, we need to wait it is gone by checking not found error.
		if (!checkerOpt.DeletedFromAPI && d.DeletionTimestamp != nil) || apierrors.IsNotFound(err) {
			return true, nil
		}
		if err == nil {
			return false, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("failed to wait backup Deployment deleted: %v", err)
	}

	_, err = WaitPodsDeleted(kubeClient, cl.Namespace, 2,
		metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"app":          k8sutil.BackupPodSelectorAppField,
				"etcd_cluster": cl.Name,
			}).String(),
		})
	if err != nil {
		return fmt.Errorf("failed to wait backup pod terminated: %v", err)
	}
	// The rest is to track backup storage, e.g. PV or S3 "dir" deleted.
	// If AutoDelete=false, we don't delete them and thus don't check them.
	if !cl.Spec.Backup.AutoDelete {
		return nil
	}
	err = retryutil.Retry(retryInterval, 3, func() (done bool, err error) {
		switch cl.Spec.Backup.StorageType {
		case api.BackupStorageTypePersistentVolume, api.BackupStorageTypeDefault:
			pl, err := kubeClient.CoreV1().PersistentVolumeClaims(cl.Namespace).List(k8sutil.ClusterListOpt(cl.Name))
			if err != nil {
				return false, err
			}
			if len(pl.Items) > 0 {
				return false, nil
			}
		case api.BackupStorageTypeS3:
			s3cli := backups3.NewFromClient(checkerOpt.S3Bucket, path.Join(cl.Namespace, cl.Name), checkerOpt.S3Cli)
			keys, err := s3cli.List()
			if err != nil {
				return false, err
			}
			if len(keys) > 0 {
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

func WaitPodsWithImageDeleted(kubecli kubernetes.Interface, namespace, image string, retries int, lo metav1.ListOptions) ([]*v1.Pod, error) {
	return waitPodsDeleted(kubecli, namespace, retries, lo, func(p *v1.Pod) bool {
		for _, c := range p.Spec.Containers {
			if c.Image == image {
				return false
			}
		}
		return true
	})
}

func WaitPodsDeleted(kubecli kubernetes.Interface, namespace string, retries int, lo metav1.ListOptions) ([]*v1.Pod, error) {
	f := func(p *v1.Pod) bool { return p.DeletionTimestamp != nil }
	return waitPodsDeleted(kubecli, namespace, retries, lo, f)
}

func WaitPodsDeletedCompletely(kubecli kubernetes.Interface, namespace string, retries int, lo metav1.ListOptions) ([]*v1.Pod, error) {
	return waitPodsDeleted(kubecli, namespace, retries, lo)
}

func waitPodsDeleted(kubecli kubernetes.Interface, namespace string, retries int, lo metav1.ListOptions, filters ...filterFunc) ([]*v1.Pod, error) {
	var pods []*v1.Pod
	err := retryutil.Retry(retryInterval, retries, func() (bool, error) {
		podList, err := kubecli.CoreV1().Pods(namespace).List(lo)
		if err != nil {
			return false, err
		}
		pods = nil
		for i := range podList.Items {
			p := &podList.Items[i]
			filtered := false
			for _, filter := range filters {
				if filter(p) {
					filtered = true
				}
			}
			if !filtered {
				pods = append(pods, p)
			}
		}
		return len(pods) == 0, nil
	})
	return pods, err
}

// WaitUntilOperatorReady will wait until the first pod selected for the label name=etcd-operator is ready.
func WaitUntilOperatorReady(kubecli kubernetes.Interface, namespace, name string) error {
	var podName string
	lo := metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(NameLabelSelector(name)).String(),
	}
	err := retryutil.Retry(10*time.Second, 6, func() (bool, error) {
		podList, err := kubecli.CoreV1().Pods(namespace).List(lo)
		if err != nil {
			return false, err
		}
		if len(podList.Items) > 0 {
			podName = podList.Items[0].Name
			if k8sutil.IsPodReady(&podList.Items[0]) {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for pod (%v) to become ready: %v", podName, err)
	}
	return nil
}
