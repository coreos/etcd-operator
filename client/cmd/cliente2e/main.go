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

package main

import (
	"context"
	"os"

	"github.com/coreos/etcd-operator/client/experimentalclient"
	"github.com/coreos/etcd-operator/pkg/spec"

	"github.com/Sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func main() {
	opcli, err := experimentalclient.NewOperator(os.Getenv("MY_POD_NAMESPACE"))
	if err != nil {
		logrus.Fatalf("fail to create operator client: %v", err)
	}
	name := "test-cluster"
	size := 3
	testCreate(opcli, name, size)
	testGet(opcli, name, size)
	size = 5
	testUpdate(opcli, name, size)
	testGet(opcli, name, size)
	testDelete(opcli, name)
	testGetNotFound(opcli, name)
}

func testCreate(opcli experimentalclient.Operator, name string, size int) {
	err := opcli.Create(context.TODO(), name, spec.ClusterSpec{
		Size: size,
	})
	if err != nil {
		logrus.Fatalf("fail to create cluster: %v", err)
	}
}

func testUpdate(opcli experimentalclient.Operator, name string, size int) {
	err := opcli.Update(context.TODO(), name, spec.ClusterSpec{
		Size: size,
	})
	if err != nil {
		logrus.Fatalf("fail to update cluster: %v", err)
	}
}

func testGet(opcli experimentalclient.Operator, name string, size int) {
	c, err := opcli.Get(context.TODO(), "test-cluster")
	if err != nil {
		logrus.Fatalf("fail to list clusters: %v", err)
	}
	if c.Metadata.Name != "test-cluster" {
		logrus.Fatalf("cluster name is wrong: %v", c.Metadata.Name)
	}
	if c.Spec.Size != size {
		logrus.Fatalf("cluster size is wrong: %v", c.Spec.Size)
	}
}

func testDelete(opcli experimentalclient.Operator, name string) {
	err := opcli.Delete(context.TODO(), "test-cluster")
	if err != nil {
		logrus.Fatalf("fail to delete cluster: %v", err)
	}
}

func testGetNotFound(opcli experimentalclient.Operator, name string) {
	_, err := opcli.Get(context.TODO(), "test-cluster")
	if err == nil || !apierrors.IsNotFound(err) {
		logrus.Fatalf("expect not found error, get: %v", err)
	}
}
