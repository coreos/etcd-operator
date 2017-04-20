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
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"github.com/coreos/etcd/embed"
	"github.com/coreos/etcd/pkg/netutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

func TestSelfHosted(t *testing.T) {
	t.Run("self hosted", func(t *testing.T) {
		t.Run("create self hosted cluster from scratch", testCreateSelfHostedCluster)
		t.Run("migrate boot member to self hosted cluster", testCreateSelfHostedClusterWithBootMember)
	})
	cleanupSelfHostedHostpath()
}

func testCreateSelfHostedCluster(t *testing.T) {
	if os.Getenv(framework.EnvCloudProvider) == "aws" {
		t.Skip("skipping test due to relying on PodIP reachability. TODO: Remove this skip later")
	}

	f := framework.Global
	c := newClusterSpec("test-etcd-", 3)
	c = clusterWithSelfHosted(c, &spec.SelfHostedPolicy{})
	testEtcd, err := createCluster(t, f, c)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(t, f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(t, f, testEtcd.Metadata.Name, 3, 240*time.Second); err != nil {
		t.Fatalf("failed to create 3 members self-hosted etcd cluster: %v", err)
	}
}

func testCreateSelfHostedClusterWithBootMember(t *testing.T) {
	if os.Getenv(framework.EnvCloudProvider) == "aws" {
		t.Skip("skipping test due to relying on PodIP reachability. TODO: Remove this skip later")
	}

	dir, err := ioutil.TempDir("", "embed-etcd")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	host, _ := netutil.GetDefaultHost()

	rand.Seed(time.Now().UnixNano())
	port := rand.Intn(61000-32768-1) + 32768 // linux ephemeral ports

	embedCfg := embed.NewConfig()
	embedCfg.Dir = dir
	lpurl, _ := url.Parse(fmt.Sprintf("http://%s:%d", host, port+1))
	lcurl, _ := url.Parse(fmt.Sprintf("http://%s:%d", host, port))
	embedCfg.LCUrls = []url.URL{*lcurl}
	embedCfg.LPUrls = []url.URL{*lpurl}

	apurl, _ := url.Parse(fmt.Sprintf("http://%s:%d", host, port+1))
	acurl, _ := url.Parse(fmt.Sprintf("http://%s:%d", host, port))
	embedCfg.ACUrls = []url.URL{*acurl}
	embedCfg.APUrls = []url.URL{*apurl}
	embedCfg.InitialCluster = "default=" + apurl.String()

	e, err := embed.StartEtcd(embedCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(30 * time.Second):
		t.Fatal("timeout: wait etcd server ready")
	}

	t.Log("etcdserver is ready")

	f := framework.Global

	c := newClusterSpec("test-etcd-", 3)
	c = clusterWithSelfHosted(c, &spec.SelfHostedPolicy{
		BootMemberClientEndpoint: embedCfg.ACUrls[0].String(),
	})
	testEtcd, err := createCluster(t, f, c)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(t, f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(t, f, testEtcd.Metadata.Name, 3, 120*time.Second); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
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
