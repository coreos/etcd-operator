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

package e2eslow

import (
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

var createTLSSecretsOnce sync.Once

func TestTLS(t *testing.T) {
	testTLS(t, false)
}

func testTLS(t *testing.T, selfHosted bool) {
	f := framework.Global
	clusterName := "tls-test"
	memberPeerTLSSecret := "etcd-server-peer-tls"
	memberClientTLSSecret := "etcd-server-client-tls"
	operatorClientTLSSecret := "operator-etcd-client-tls"
	createTLSSecretsOnce.Do(func() {
		err := e2eutil.PreparePeerTLSSecret(clusterName, f.Namespace, memberPeerTLSSecret)
		if err != nil {
			t.Fatal(err)
		}
		certsDir, err := ioutil.TempDir("", "etcd-operator-tls-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(certsDir)
		err = e2eutil.PrepareClientTLSSecret(certsDir, clusterName, f.Namespace, memberClientTLSSecret, operatorClientTLSSecret)
		if err != nil {
			t.Fatal(err)
		}
	})

	c := e2eutil.NewCluster("", 3)
	c.Metadata.Name = clusterName
	c.Spec.TLS = &spec.TLSPolicy{
		Static: &spec.StaticTLS{
			Member: &spec.MemberSecret{
				PeerSecret:   memberPeerTLSSecret,
				ClientSecret: memberClientTLSSecret,
			},
			OperatorSecret: operatorClientTLSSecret,
		},
	}
	if selfHosted {
		c = e2eutil.ClusterWithSelfHosted(c, &spec.SelfHostedPolicy{})
	}
	c, err := e2eutil.CreateCluster(t, f.KubeClient, f.Namespace, c)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteCluster(t, f.KubeClient, c); err != nil {
			t.Fatal(err)
		}
	}()

	_, err = e2eutil.WaitUntilSizeReached(t, f.KubeClient, 3, 60*time.Second, c)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	// TODO: use client key/certs to talk to secure etcd cluster.
}
