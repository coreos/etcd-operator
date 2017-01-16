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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/cluster"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/coreos/etcd/clientv3"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	k8sclient "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

const (
	envParallelTest     = "PARALLEL_TEST"
	envParallelTestTrue = "true"

	etcdKeyFoo = "foo"
	etcdValBar = "bar"
)

func waitBackupPodUp(f *framework.Framework, clusterName string, timeout time.Duration) error {
	return retryutil.Retry(5*time.Second, int(timeout/(5*time.Second)), func() (done bool, err error) {
		podList, err := f.KubeClient.Pods(f.Namespace).List(api.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"app":          k8sutil.BackupPodSelectorAppField,
				"etcd_cluster": clusterName,
			})})
		if err != nil {
			return false, err
		}
		for i := range podList.Items {
			if podList.Items[i].Status.Phase == api.PodRunning {
				return true, nil
			}
		}
		return false, nil
	})
}

func makeBackup(f *framework.Framework, clusterName string) error {
	ls := map[string]string{
		"app":          k8sutil.BackupPodSelectorAppField,
		"etcd_cluster": clusterName,
	}
	podList, err := f.KubeClient.Pods(f.Namespace).List(api.ListOptions{
		LabelSelector: labels.SelectorFromSet(ls),
	})
	if err != nil {
		return err
	}
	if len(podList.Items) < 1 {
		return fmt.Errorf("no backup pod found")
	}

	// We are assuming pod ip is accessible from test machine.
	addr := fmt.Sprintf("%s:%d", podList.Items[0].Status.PodIP, constants.DefaultBackupPodHTTPPort)
	err = cluster.RequestBackupNow(f.KubeClient.Client, addr)
	if err != nil {
		return fmt.Errorf("backup pod (%s): %v", podList.Items[0].Name, err)
	}
	return nil
}

func waitUntilSizeReached(t *testing.T, f *framework.Framework, clusterName string, size int, timeout time.Duration) ([]string, error) {
	return waitSizeReachedWithAccept(t, f, clusterName, size, timeout, func(*api.Pod) bool { return true })
}

func waitSizeReachedWithAccept(t *testing.T, f *framework.Framework, clusterName string, size int, timeout time.Duration, acceptPod func(*api.Pod) bool) ([]string, error) {
	var names []string
	err := retryutil.Retry(10*time.Second, int(timeout/(10*time.Second)), func() (done bool, err error) {
		podList, err := f.KubeClient.Pods(f.Namespace).List(k8sutil.ClusterListOpt(clusterName))
		if err != nil {
			return false, err
		}
		names = nil
		for i := range podList.Items {
			pod := &podList.Items[i]
			if pod.Status.Phase != api.PodRunning || !acceptPod(pod) {
				continue
			}
			names = append(names, pod.Name)
		}
		logfWithTimestamp(t, "waiting size (%d), etcd pods: %v", size, names)
		if len(names) != size {
			return false, nil
		}
		// TODO: check etcd member membership
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

func killMembers(f *framework.Framework, names ...string) error {
	for _, name := range names {
		err := f.KubeClient.Pods(f.Namespace).Delete(name, api.NewDeleteOptions(0))
		if err != nil && !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func makeEtcdCluster(genName string, size int) *spec.EtcdCluster {
	return &spec.EtcdCluster{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "EtcdCluster",
			APIVersion: "coreos.com/v1",
		},
		ObjectMeta: api.ObjectMeta{
			GenerateName: genName,
		},
		Spec: &spec.ClusterSpec{
			Size: size,
		},
	}
}

func makeBackupPolicy(cleanup bool) *spec.BackupPolicy {
	return &spec.BackupPolicy{
		BackupIntervalInSecond:        60 * 60,
		MaxBackups:                    5,
		VolumeSizeInMB:                512,
		StorageType:                   spec.BackupStorageTypeDefault,
		CleanupBackupsOnClusterDelete: cleanup,
	}
}

func backupPolicyWithStorageType(bp *spec.BackupPolicy, bt spec.BackupStorageType) *spec.BackupPolicy {
	bp.StorageType = bt
	return bp
}

func etcdClusterWithBackup(ec *spec.EtcdCluster, backupPolicy *spec.BackupPolicy) *spec.EtcdCluster {
	ec.Spec.Backup = backupPolicy
	return ec
}

func etcdClusterWithRestore(ec *spec.EtcdCluster, restorePolicy *spec.RestorePolicy) *spec.EtcdCluster {
	ec.Spec.Restore = restorePolicy
	return ec
}

func etcdClusterWithVersion(ec *spec.EtcdCluster, version string) *spec.EtcdCluster {
	ec.Spec.Version = version
	return ec
}

func createEtcdCluster(t *testing.T, f *framework.Framework, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	resp, err := f.KubeClient.Client.Post(
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters", f.MasterHost, f.Namespace),
		"application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status: %v", resp.Status)
	}
	decoder := json.NewDecoder(resp.Body)
	res := &spec.EtcdCluster{}
	if err := decoder.Decode(res); err != nil {
		return nil, err
	}
	logfWithTimestamp(t, "created etcd cluster: %v", res.Name)
	return res, nil
}

func updateEtcdCluster(f *framework.Framework, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	return k8sutil.UpdateClusterTPRObjectUnconditionally(f.KubeClient, f.MasterHost, f.Namespace, e)
}

func deleteEtcdCluster(t *testing.T, f *framework.Framework, e *spec.EtcdCluster) error {
	podList, err := f.KubeClient.Pods(f.Namespace).List(k8sutil.ClusterListOpt(e.Name))
	if err != nil {
		return err
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		t.Logf("pod (%v): status (%v), cmd (%v)", pod.Name, pod.Status.Phase, pod.Spec.Containers[0].Command)
	}

	buf := bytes.NewBuffer(nil)
	if err := getLogs(f.KubeClient, f.Namespace, "etcd-operator", "etcd-operator", buf); err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE",
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters/%s", f.MasterHost, f.Namespace, e.Name), nil)
	if err != nil {
		return err
	}
	resp, err := f.KubeClient.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %v", resp.Status)
	}
	return waitResourcesDeleted(t, f, e)
}

func waitResourcesDeleted(t *testing.T, f *framework.Framework, e *spec.EtcdCluster) error {
	err := retryutil.Retry(5*time.Second, 10, func() (done bool, err error) {
		list, err := f.KubeClient.Pods(f.Namespace).List(k8sutil.ClusterListOpt(e.Name))
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

	err = retryutil.Retry(5*time.Second, 10, func() (done bool, err error) {
		list, err := f.KubeClient.Services(f.Namespace).List(k8sutil.ClusterListOpt(e.Name))
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

	if e.Spec.Backup != nil {
		err := waitBackupDeleted(f, e)
		if err != nil {
			return fmt.Errorf("fail to wait backup deleted: %v", err)
		}
	}
	return nil
}

func waitBackupDeleted(f *framework.Framework, e *spec.EtcdCluster) error {
	err := retryutil.Retry(5*time.Second, 10, func() (done bool, err error) {
		rl, err := f.KubeClient.ReplicaSets(f.Namespace).List(k8sutil.ClusterListOpt(e.Name))
		if err != nil {
			return false, err
		}
		if len(rl.Items) > 0 {
			return false, nil
		}
		// TODO: check backup pod deleted. There is a graceful deletion that takes too long
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait backup RS deleted: %v", err)
	}
	// The rest is to track backup storage, e.g. PV or S3 "dir" deleted.
	// If CleanupBackupsOnClusterDelete=false, we don't delete them and thus don't check them.
	if !e.Spec.Backup.CleanupBackupsOnClusterDelete {
		return nil
	}
	err = retryutil.Retry(5*time.Second, 10, func() (done bool, err error) {
		switch e.Spec.Backup.StorageType {
		case spec.BackupStorageTypePersistentVolume, spec.BackupStorageTypeDefault:
			pl, err := f.KubeClient.PersistentVolumeClaims(f.Namespace).List(k8sutil.ClusterListOpt(e.Name))
			if err != nil {
				return false, err
			}
			if len(pl.Items) > 0 {
				return false, nil
			}
		case spec.BackupStorageTypeS3:
			resp, err := f.S3Cli.ListObjects(&s3.ListObjectsInput{
				Bucket: aws.String(f.S3Bucket),
				Prefix: aws.String(e.Name + "/"),
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
		return fmt.Errorf("failed to wait storage (%s) to be deleted: %v", e.Spec.Backup.StorageType, err)
	}
	return nil
}

func getLogs(kubecli *k8sclient.Client, ns, p, c string, out io.Writer) error {
	req := kubecli.RESTClient.Get().
		Namespace(ns).
		Resource("pods").
		Name(p).
		SubResource("log").
		Param("container", c).
		Param("tailLines", "20")

	readCloser, err := req.Stream()
	if err != nil {
		return err
	}
	defer readCloser.Close()

	_, err = io.Copy(out, readCloser)
	return err
}

func printContainerStatus(buf *bytes.Buffer, ss []api.ContainerStatus) {
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
