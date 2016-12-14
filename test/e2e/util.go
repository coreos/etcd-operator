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
	"time"

	"github.com/coreos/etcd-operator/pkg/cluster"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"github.com/coreos/etcd/clientv3"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	k8sclient "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/wait"
)

const (
	etcdKeyFoo = "foo"
	etcdValBar = "bar"
)

func waitBackupPodUp(f *framework.Framework, clusterName string, timeout time.Duration) error {
	return wait.Poll(5*time.Second, timeout, func() (done bool, err error) {
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
	return cluster.RequestBackupNow(f.KubeClient.Client, addr)
}

func waitUntilSizeReached(f *framework.Framework, clusterName string, size, timeout int) ([]string, error) {
	return waitSizeReachedWithFilter(f, clusterName, size, timeout, func(*api.Pod) bool { return true })
}

func waitSizeReachedWithFilter(f *framework.Framework, clusterName string, size, timeout int, filterPod func(*api.Pod) bool) ([]string, error) {
	var names []string
	err := wait.Poll(10*time.Second, time.Duration(timeout)*time.Second, func() (done bool, err error) {
		podList, err := f.KubeClient.Pods(f.Namespace).List(k8sutil.EtcdPodListOpt(clusterName))
		if err != nil {
			return false, err
		}
		names = nil
		for i := range podList.Items {
			pod := &podList.Items[i]
			if pod.Status.Phase == api.PodRunning {
				names = append(names, pod.Name)
			}
		}
		fmt.Printf("waiting size (%d), etcd pods: %v\n", size, names)
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
		if err != nil {
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
		Spec: spec.ClusterSpec{
			Size: size,
		},
	}
}

func makeBackupPolicy(cleanup bool) *spec.BackupPolicy {
	return &spec.BackupPolicy{
		SnapshotIntervalInSecond: 60 * 60,
		MaxSnapshot:              5,
		VolumeSizeInMB:           512,
		StorageType:              spec.BackupStorageTypePersistentVolume,
		CleanupOnClusterDelete:   cleanup,
	}
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

func createEtcdCluster(f *framework.Framework, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
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
	fmt.Printf("created etcd cluster: %v\n", res.Name)
	return res, nil
}

func updateEtcdCluster(f *framework.Framework, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("PUT",
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters/%s", f.MasterHost, f.Namespace, e.Name),
		bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.KubeClient.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %v", resp.Status)
	}
	decoder := json.NewDecoder(resp.Body)
	nspec := &spec.EtcdCluster{}
	if err := decoder.Decode(nspec); err != nil {
		return nil, err
	}
	return nspec, nil
}

func deleteEtcdCluster(f *framework.Framework, name string) error {
	fmt.Printf("deleting etcd cluster: %v\n", name)
	podList, err := f.KubeClient.Pods(f.Namespace).List(k8sutil.EtcdPodListOpt(name))
	if err != nil {
		return err
	}
	fmt.Println("etcd pods ======")
	for i := range podList.Items {
		pod := &podList.Items[i]
		fmt.Printf("pod (%v): status (%v)\n", pod.Name, pod.Status.Phase)
		buf := bytes.NewBuffer(nil)

		if pod.Status.Phase == api.PodFailed {
			if err := getLogs(f.KubeClient, f.Namespace, pod.Name, "etcd", buf); err != nil {
				return err
			}
			fmt.Println(pod.Name, "logs ===")
			fmt.Println(buf.String())
			fmt.Println(pod.Name, "logs END ===")
		}
	}

	buf := bytes.NewBuffer(nil)
	if err := getLogs(f.KubeClient, f.Namespace, "etcd-operator", "etcd-operator", buf); err != nil {
		return err
	}
	fmt.Println("etcd-operator logs ===")
	fmt.Println(buf.String())
	fmt.Println("etcd-operator logs END ===")

	req, err := http.NewRequest("DELETE",
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters/%s", f.MasterHost, f.Namespace, name), nil)
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
