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

package e2esh

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eslow"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

func TestSelfHosted(t *testing.T) {
	t.Run("create self hosted cluster from scratch", testCreateSelfHostedCluster)
	t.Run("migrate boot member to self hosted cluster", testCreateSelfHostedClusterWithBootMember)
	t.Run("backup for self hosted cluster", testSelfHostedClusterWithBackup)
	t.Run("TLS for self hosted cluster", func(t *testing.T) { e2eslow.TLSTestCommon(t, true) })
	cleanupSelfHostedHostpath()
}

func testCreateSelfHostedCluster(t *testing.T) {
	f := framework.Global
	c := e2eutil.NewCluster("test-etcd-", 3)
	c = e2eutil.ClusterWithSelfHosted(c, &spec.SelfHostedPolicy{})
	testEtcd, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, c)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 24, testEtcd); err != nil {
		t.Fatalf("failed to create 3 members self-hosted etcd cluster: %v", err)
	}
}

func testCreateSelfHostedClusterWithBootMember(t *testing.T) {
	f := framework.Global

	bootEtcdPod, err := startEtcd(f)
	if err != nil {
		t.Fatal(err)
	}
	defer f.KubeClient.CoreV1().Pods(f.Namespace).Delete(bootEtcdPod.Name, metav1.NewDeleteOptions(0))

	bootURL := fmt.Sprintf("http://%s:2379", bootEtcdPod.Status.PodIP)

	t.Logf("boot etcd URL: %s", bootURL)

	c := e2eutil.NewCluster("test-etcd-", 3)
	c = e2eutil.ClusterWithSelfHosted(c, &spec.SelfHostedPolicy{
		BootMemberClientEndpoint: bootURL,
	})
	testEtcd, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, c)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 12, testEtcd); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
}

var etcdCmd = `
  etcd --name=$(POD_NAME) --data-dir=/var/etcd/data \
  --listen-client-urls=http://0.0.0.0:2379 --listen-peer-urls=http://0.0.0.0:2380 \
  --advertise-client-urls=http://$(POD_IP):2379 --initial-advertise-peer-urls=http://$(POD_IP):2380 \
  --initial-cluster=$(POD_NAME)=http://$(POD_IP):2380 --initial-cluster-token=$(POD_NAME)  --initial-cluster-state=new
`

func startEtcd(f *framework.Framework) (*v1.Pod, error) {
	p := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "boot-etcd",
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{{
				Command: []string{"/bin/sh", "-ec", etcdCmd},
				Name:    "etcd",
				Image:   "quay.io/coreos/etcd:v3.1.8",
				Env: []v1.EnvVar{{
					Name:      "POD_NAME",
					ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"}},
				}, {
					Name:      "POD_IP",
					ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "status.podIP"}},
				}},
			}},
		},
	}
	return k8sutil.CreateAndWaitPod(f.KubeClient, f.Namespace, p, 30*time.Second)
}

func testSelfHostedClusterWithBackup(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}

	f := framework.Global

	cl := e2eutil.NewCluster("test-cluster-", 3)
	cl = e2eutil.ClusterWithBackup(cl, e2eutil.NewS3BackupPolicy(true))
	cl = e2eutil.ClusterWithSelfHosted(cl, &spec.SelfHostedPolicy{})

	testEtcd, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, cl)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		storageCheckerOptions := e2eutil.StorageCheckerOptions{
			S3Cli:    f.S3Cli,
			S3Bucket: f.S3Bucket,
		}
		err := e2eutil.DeleteClusterAndBackup(t, f.CRClient, f.KubeClient, testEtcd, storageCheckerOptions)
		if err != nil {
			t.Fatal(err)
		}
	}()

	_, err = e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 6, testEtcd)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	fmt.Println("reached to 3 members cluster")
	err = e2eutil.WaitBackupPodUp(t, f.KubeClient, f.Namespace, testEtcd.Name, 6)
	if err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}
	err = e2eutil.MakeBackup(f.KubeClient, f.Namespace, testEtcd.Name)
	if err != nil {
		t.Fatalf("fail to make a latest backup: %v", err)
	}
}

func cleanupSelfHostedHostpath() {
	f := framework.Global
	nodes, err := f.KubeClient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	for i := range nodes.Items {
		wg.Add(1)
		go func(nodeName string) {
			defer wg.Done()

			name := fmt.Sprintf("cleanup-selfhosted-%s", nodeName)
			p := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: v1.PodSpec{
					NodeName:      nodeName,
					RestartPolicy: v1.RestartPolicyOnFailure,
					Containers: []v1.Container{{
						Name:  name,
						Image: "busybox",
						VolumeMounts: []v1.VolumeMount{
							// TODO: This is an assumption on etcd container volume mount.
							{Name: "etcd-data", MountPath: "/var/etcd"},
						},
						Command: []string{
							// TODO: this is an assumption on the format of etcd data dir.
							"/bin/sh", "-ec", fmt.Sprintf("rm -rf /var/etcd/%s-*", f.Namespace),
						},
					}},
					Volumes: []v1.Volume{{
						Name: "etcd-data",
						// TODO: This is an assumption on self hosted etcd volumes.
						VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: "/var/etcd"}},
					}},
				},
			}
			_, err := f.KubeClient.CoreV1().Pods(f.Namespace).Create(p)
			if err != nil {
				return
			}
			retryutil.Retry(5*time.Second, 5, func() (bool, error) {
				get, err := f.KubeClient.CoreV1().Pods(f.Namespace).Get(name, metav1.GetOptions{})
				if err != nil {
					return false, nil
				}
				ph := get.Status.Phase
				return ph == v1.PodSucceeded || ph == v1.PodFailed, nil
			})
			f.KubeClient.CoreV1().Pods(f.Namespace).Delete(name, nil)
		}(nodes.Items[i].Name)
	}
	wg.Wait()
}
