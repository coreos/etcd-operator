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

package upgradetest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

func TestResize(t *testing.T) {
	defer func() {
		err := testF.DeleteOperator()
		if err != nil {
			t.Fatal(err)
		}
	}()
	err := testF.CreateOperator()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Second)
	testClus, err := e2eutil.CreateCluster(t, testF.KubeCli, testF.KubeNS, newClusterSpec())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := e2eutil.DeleteEtcdCluster(t, testF.KubeCli, testClus, &e2eutil.StorageCheckerOptions{}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Second)
	}()
	_, err = waitUntilSizeReached(testClus.Metadata.Name, 3, 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	err = testF.UpgradeOperator()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)

	testClus, err = k8sutil.GetClusterTPRObject(testF.KubeCli.CoreV1().RESTClient(), testF.KubeNS, testClus.Metadata.Name)
	if err != nil {
		t.Fatal(err)
	}

	updateFunc := func(cl *spec.Cluster) {
		cl.Spec.Size = 5
	}
	if _, err := e2eutil.UpdateEtcdCluster(testF.KubeCli, testClus, 10, updateFunc); err != nil {
		t.Fatal(err)
	}
	_, err = waitUntilSizeReached(testClus.Metadata.Name, 5, 60*time.Second)
	if err != nil {
		t.Fatal(err)
	}
}

func newClusterSpec() *spec.Cluster {
	return &spec.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       strings.Title(spec.TPRKind),
			APIVersion: spec.TPRGroup + "/" + spec.TPRVersion,
		},
		Metadata: metav1.ObjectMeta{
			Name: "upgrade-test",
		},
		Spec: spec.ClusterSpec{
			Size: 3,
		},
	}
}

func waitUntilSizeReached(clusterName string, size int, timeout time.Duration) ([]string, error) {
	var names []string
	err := retryutil.Retry(10*time.Second, int(timeout/(10*time.Second)), func() (bool, error) {
		podList, err := testF.KubeCli.CoreV1().Pods(testF.KubeNS).List(k8sutil.ClusterListOpt(clusterName))
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
		fmt.Println("names:", names)
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
