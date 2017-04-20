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
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
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

func createCluster(t *testing.T, f *framework.Framework, cl *spec.Cluster) (*spec.Cluster, error) {
	uri := fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters", spec.TPRGroup, spec.TPRVersion, f.Namespace)
	b, err := f.KubeClient.CoreV1().RESTClient().Post().Body(cl).RequestURI(uri).DoRaw()
	if err != nil {
		return nil, err
	}
	res := &spec.Cluster{}
	if err := json.Unmarshal(b, res); err != nil {
		return nil, err
	}
	logfWithTimestamp(t, "created etcd cluster: %v", res.Metadata.Name)
	return res, nil
}

func updateEtcdCluster(f *framework.Framework, c *spec.Cluster) (*spec.Cluster, error) {
	return k8sutil.UpdateClusterTPRObjectUnconditionally(f.KubeClient.CoreV1().RESTClient(), f.Namespace, c)
}

func deleteEtcdCluster(t *testing.T, f *framework.Framework, c *spec.Cluster) error {
	podList, err := f.KubeClient.CoreV1().Pods(f.Namespace).List(k8sutil.ClusterListOpt(c.Metadata.Name))
	if err != nil {
		return err
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		t.Logf("pod (%v): status (%v), node (%v) cmd (%v)", pod.Name, pod.Status.Phase, pod.Spec.NodeName, pod.Spec.Containers[0].Command)
	}

	uri := fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters/%s", spec.TPRGroup, spec.TPRVersion, f.Namespace, c.Metadata.Name)
	if _, err := f.KubeClient.CoreV1().RESTClient().Delete().RequestURI(uri).DoRaw(); err != nil {
		return err
	}
	return waitResourcesDeleted(t, f, c)
}

func waitResourcesDeleted(t *testing.T, f *framework.Framework, c *spec.Cluster) error {
	err := retryutil.Retry(5*time.Second, 5, func() (done bool, err error) {
		list, err := f.KubeClient.CoreV1().Pods(f.Namespace).List(k8sutil.ClusterListOpt(c.Metadata.Name))
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
		list, err := f.KubeClient.CoreV1().Services(f.Namespace).List(k8sutil.ClusterListOpt(c.Metadata.Name))
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

	if c.Spec.Backup != nil {
		err := waitBackupDeleted(f, c)
		if err != nil {
			return fmt.Errorf("fail to wait backup deleted: %v", err)
		}
	}
	return nil
}

func waitBackupDeleted(f *framework.Framework, c *spec.Cluster) error {
	err := retryutil.Retry(5*time.Second, 5, func() (done bool, err error) {
		rl, err := f.KubeClient.Extensions().ReplicaSets(f.Namespace).List(k8sutil.ClusterListOpt(c.Metadata.Name))
		if err != nil {
			return false, err
		}
		if len(rl.Items) > 0 {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait backup RS deleted: %v", err)
	}
	err = retryutil.Retry(5*time.Second, 2, func() (done bool, err error) {
		ls := labels.SelectorFromSet(map[string]string{
			"app":          k8sutil.BackupPodSelectorAppField,
			"etcd_cluster": c.Metadata.Name,
		}).String()
		pl, err := f.KubeClient.CoreV1().Pods(f.Namespace).List(metav1.ListOptions{
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
	if !c.Spec.Backup.CleanupBackupsOnClusterDelete {
		return nil
	}
	err = retryutil.Retry(5*time.Second, 5, func() (done bool, err error) {
		switch c.Spec.Backup.StorageType {
		case spec.BackupStorageTypePersistentVolume, spec.BackupStorageTypeDefault:
			pl, err := f.KubeClient.CoreV1().PersistentVolumeClaims(f.Namespace).List(k8sutil.ClusterListOpt(c.Metadata.Name))
			if err != nil {
				return false, err
			}
			if len(pl.Items) > 0 {
				return false, nil
			}
		case spec.BackupStorageTypeS3:
			resp, err := f.S3Cli.ListObjects(&s3.ListObjectsInput{
				Bucket: aws.String(f.S3Bucket),
				Prefix: aws.String(c.Metadata.Name + "/"),
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
		return fmt.Errorf("failed to wait storage (%s) to be deleted: %v", c.Spec.Backup.StorageType, err)
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
