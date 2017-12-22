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
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func TestTLS(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	f := framework.Global
	suffix := fmt.Sprintf("-%d", rand.Uint64())
	clusterName := "tls-test" + suffix
	memberPeerTLSSecret := "etcd-peer-tls" + suffix
	memberClientTLSSecret := "etcd-server-tls" + suffix
	operatorClientTLSSecret := "etcd-client-tls" + suffix

	err := e2eutil.PrepareTLS(clusterName, f.Namespace, memberPeerTLSSecret, memberClientTLSSecret, operatorClientTLSSecret)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err := e2eutil.DeleteSecrets(f.KubeClient, f.Namespace, memberPeerTLSSecret, memberClientTLSSecret, operatorClientTLSSecret)
		if err != nil {
			t.Fatal(err)
		}
	}()

	c := e2eutil.NewCluster("", 3)
	c.Name = clusterName
	e2eutil.ClusterCRWithTLS(c, memberPeerTLSSecret, memberClientTLSSecret, operatorClientTLSSecret)
	c, err = e2eutil.CreateCluster(t, f.CRClient, f.Namespace, c)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, c); err != nil {
			t.Fatal(err)
		}
	}()

	_, err = e2eutil.WaitUntilSizeReached(t, f.CRClient, 3, 6, c)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	// TODO: use client key/certs to talk to secure etcd cluster.
}
