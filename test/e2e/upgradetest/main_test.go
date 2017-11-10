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
	"flag"
	"os"
	"testing"

	"github.com/coreos/etcd-operator/test/e2e/upgradetest/framework"

	"github.com/sirupsen/logrus"
)

var testF *framework.Framework

func TestMain(m *testing.M) {
	kubeconfig := flag.String("kubeconfig", "", "kube config path, e.g. $HOME/.kube/config")
	kubeNS := flag.String("kube-ns", "default", "upgrade test namespace")
	oldImage := flag.String("old-image", "", "")
	newImage := flag.String("new-image", "", "")
	flag.Parse()

	cfg := framework.Config{
		KubeConfig: *kubeconfig,
		KubeNS:     *kubeNS,
		OldImage:   *oldImage,
		NewImage:   *newImage,
	}
	var err error
	testF, err = framework.New(cfg)
	if err != nil {
		logrus.Fatalf("failed to create test framework: %v", err)
	}

	code := m.Run()
	os.Exit(code)
}
