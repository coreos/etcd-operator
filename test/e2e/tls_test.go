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

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPeerTLS(t *testing.T) {
	f := framework.Global
	clusterName := "peer-tls-test"
	secretName := "etcd-server-peer-tls"
	err := e2eutil.PreparePeerTLSSecret(clusterName, f.Namespace, secretName)
	if err != nil {
		t.Fatal(err)
	}

	c := newClusterSpec("", 3)
	c.Metadata.Name = clusterName
	c.Spec.TLS = &spec.TLSPolicy{
		Static: &spec.StaticTLS{
			Member: &spec.MemberSecret{
				PeerSecret: secretName,
			},
		},
	}
	c, err = createCluster(t, f, c)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(t, f, c); err != nil {
			t.Fatal(err)
		}
	}()

	members, err := waitUntilSizeReached(t, f, c.Metadata.Name, 3, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	pod, err := f.KubeClient.CoreV1().Pods(f.Namespace).Get(members[0], metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// TODO: get rid of pod IP assumption.
	clientURL := fmt.Sprintf("http://%s:2379", pod.Status.PodIP)
	err = putDataToEtcd(clientURL)
	if err != nil {
		t.Fatal(err)
	}
	checkEtcdData(t, clientURL)
}
