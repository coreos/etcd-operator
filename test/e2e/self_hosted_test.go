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
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/test/e2e/framework"
	"github.com/coreos/etcd/embed"
	"github.com/coreos/etcd/pkg/netutil"
)

func TestSelfHosted(t *testing.T) {
	t.Run("self hosted", func(t *testing.T) {
		t.Run("create self hosted cluster from scratch", testCreateSelfHostedCluster)
		t.Run("migrate boot member to self hosted cluster", testCreateSelfHostedClusterWithBootMember)
	})
}

func testCreateSelfHostedCluster(t *testing.T) {
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
