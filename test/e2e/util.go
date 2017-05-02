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

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/client/experimentalclient"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"github.com/coreos/etcd/clientv3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	envParallelTest     = "PARALLEL_TEST"
	envParallelTestTrue = "true"

	etcdKeyFoo = "foo"
	etcdValBar = "bar"
)

type acceptFunc func(*v1.Pod) bool

func waitBackupPodUp(f *framework.Framework, clusterName string, timeout time.Duration) error {
	ls := labels.SelectorFromSet(map[string]string{
		"app":          k8sutil.BackupPodSelectorAppField,
		"etcd_cluster": clusterName,
	}).String()
	return retryutil.Retry(5*time.Second, int(timeout/(5*time.Second)), func() (done bool, err error) {
		podList, err := f.KubeClient.CoreV1().Pods(f.Namespace).List(metav1.ListOptions{
			LabelSelector: ls,
		})
		if err != nil {
			return false, err
		}
		for i := range podList.Items {
			if podList.Items[i].Status.Phase == v1.PodRunning {
				return true, nil
			}
		}
		return false, nil
	})
}

func makeBackup(f *framework.Framework, clusterName string) error {
	ls := labels.SelectorFromSet(map[string]string{
		"app":          k8sutil.BackupPodSelectorAppField,
		"etcd_cluster": clusterName,
	}).String()
	podList, err := f.KubeClient.CoreV1().Pods(f.Namespace).List(metav1.ListOptions{
		LabelSelector: ls,
	})
	if err != nil {
		return err
	}
	if len(podList.Items) < 1 {
		return fmt.Errorf("no backup pod found")
	}

	// We are assuming Kubernetes pod network is accessible from test machine.
	// TODO: remove this assumption.
	addr := fmt.Sprintf("%s:%d", podList.Items[0].Status.PodIP, constants.DefaultBackupPodHTTPPort)
	bc := experimentalclient.NewBackupWithAddr(&http.Client{}, "http", addr)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = bc.Request(ctx)
	if err != nil {
		return fmt.Errorf("backup pod (%s): %v", podList.Items[0].Name, err)
	}
	return nil
}

func waitUntilSizeReached(t *testing.T, f *framework.Framework, clusterName string, size int, timeout time.Duration) ([]string, error) {
	return waitSizeReachedWithAccept(t, f, clusterName, size, timeout)
}

func waitSizeAndVersionReached(t *testing.T, f *framework.Framework, clusterName, version string, size int, timeout time.Duration) error {
	_, err := waitSizeReachedWithAccept(t, f, clusterName, size, timeout, func(pod *v1.Pod) bool {
		return k8sutil.GetEtcdVersion(pod) == version
	})
	return err
}

func waitSizeReachedWithAccept(t *testing.T, f *framework.Framework, clusterName string, size int, timeout time.Duration, accepts ...acceptFunc) ([]string, error) {
	var names []string
	err := retryutil.Retry(10*time.Second, int(timeout/(10*time.Second)), func() (done bool, err error) {
		podList, err := f.KubeClient.Core().Pods(f.Namespace).List(k8sutil.ClusterListOpt(clusterName))
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
			accepted := true
			for _, acceptPod := range accepts {
				if !acceptPod(pod) {
					accepted = false
					break
				}
			}
			if !accepted {
				continue
			}
			names = append(names, pod.Name)
			nodeNames = append(nodeNames, pod.Spec.NodeName)
		}
		logfWithTimestamp(t, "waiting size (%d), etcd pods: names (%v), nodes (%v)", size, names, nodeNames)
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

func killMembers(f *framework.Framework, names ...string) error {
	for _, name := range names {
		err := f.KubeClient.CoreV1().Pods(f.Namespace).Delete(name, metav1.NewDeleteOptions(0))
		if err != nil && !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func newClusterSpec(genName string, size int) *spec.Cluster {
	return &spec.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       strings.Title(spec.TPRKind),
			APIVersion: spec.TPRGroup + "/" + spec.TPRVersion,
		},
		Metadata: metav1.ObjectMeta{
			GenerateName: genName,
		},
		Spec: spec.ClusterSpec{
			Size: size,
		},
	}
}

func etcdClusterWithBackup(cl *spec.Cluster, backupPolicy *spec.BackupPolicy) *spec.Cluster {
	cl.Spec.Backup = backupPolicy
	return cl
}

func newBackupPolicyS3() *spec.BackupPolicy {
	return &spec.BackupPolicy{
		BackupIntervalInSecond: 60 * 60,
		MaxBackups:             5,
		StorageType:            spec.BackupStorageTypeS3,
		StorageSource: spec.StorageSource{
			S3: &spec.S3Source{},
		},
	}
}

func newBackupPolicyPV() *spec.BackupPolicy {
	return &spec.BackupPolicy{
		BackupIntervalInSecond: 60 * 60,
		MaxBackups:             5,
		StorageType:            spec.BackupStorageTypePersistentVolume,
		StorageSource: spec.StorageSource{
			PV: &spec.PVSource{
				VolumeSizeInMB: 512,
			},
		},
	}
}

func etcdClusterWithRestore(cl *spec.Cluster, restorePolicy *spec.RestorePolicy) *spec.Cluster {
	cl.Spec.Restore = restorePolicy
	return cl
}

func etcdClusterWithVersion(cl *spec.Cluster, version string) *spec.Cluster {
	cl.Spec.Version = version
	return cl
}

func clusterWithSelfHosted(cl *spec.Cluster, sh *spec.SelfHostedPolicy) *spec.Cluster {
	cl.Spec.SelfHosted = sh
	return cl
}

func logfWithTimestamp(t *testing.T, format string, args ...interface{}) {
	t.Log(time.Now(), fmt.Sprintf(format, args...))
}

func createEtcdClient(addr string) (*clientv3.Client, error) {
	cfg := clientv3.Config{
		Endpoints:   []string{addr},
		DialTimeout: constants.DefaultDialTimeout,
	}
	c, err := clientv3.New(cfg)
	if err != nil {
		return nil, err
	}
	return c, nil
}
