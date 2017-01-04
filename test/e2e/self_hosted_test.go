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
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"

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
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	f := framework.Global
	testEtcd, err := createEtcdCluster(f, makeSelfHostedEnabledCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 240*time.Second); err != nil {
		t.Fatalf("failed to create 3 members self-hosted etcd cluster: %v", err)
	}
}

func testCreateSelfHostedClusterWithBootMember(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	dir := path.Join(os.TempDir(), fmt.Sprintf("embed-etcd"))
	os.RemoveAll(dir)
	defer os.RemoveAll(dir)

	host, _ := netutil.GetDefaultHost()

	embedCfg := embed.NewConfig()
	embedCfg.Dir = dir
	lpurl, _ := url.Parse("http://" + host + ":12380")
	lcurl, _ := url.Parse("http://" + host + ":12379")
	embedCfg.LCUrls = []url.URL{*lcurl}
	embedCfg.LPUrls = []url.URL{*lpurl}

	apurl, _ := url.Parse("http://" + host + ":12380")
	acurl, _ := url.Parse("http://" + host + ":12379")
	embedCfg.ACUrls = []url.URL{*acurl}
	embedCfg.APUrls = []url.URL{*apurl}
	embedCfg.InitialCluster = "default=" + apurl.String()

	e, err := embed.StartEtcd(embedCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	<-e.Server.ReadyNotify()

	t.Log("etcdserver is ready")

	f := framework.Global

	c := &spec.EtcdCluster{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "EtcdCluster",
			APIVersion: "coreos.com/v1",
		},
		ObjectMeta: api.ObjectMeta{
			GenerateName: "etcd-test-seed-",
		},
		Spec: &spec.ClusterSpec{
			Size: 3,
			SelfHosted: &spec.SelfHostedPolicy{
				BootMemberClientEndpoint: embedCfg.ACUrls[0].String(),
			},
		},
	}

	testEtcd, err := createEtcdCluster(f, c)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := deleteEtcdCluster(f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := waitUntilSizeReached(f, testEtcd.Name, 3, 120*time.Second); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
}

func makeSelfHostedEnabledCluster(genName string, size int) *spec.EtcdCluster {
	return &spec.EtcdCluster{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "EtcdCluster",
			APIVersion: "coreos.com/v1",
		},
		ObjectMeta: api.ObjectMeta{
			GenerateName: genName,
		},
		Spec: &spec.ClusterSpec{
			Size:       size,
			SelfHosted: &spec.SelfHostedPolicy{},
		},
	}
}
