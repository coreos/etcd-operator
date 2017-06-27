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

package controller

import (
	"os"

	"github.com/coreos/etcd-operator/pkg/backup/abs/absconfig"
	"github.com/coreos/etcd-operator/pkg/backup/env"
	"github.com/coreos/etcd-operator/pkg/spec"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func setupABSEnv(kubecli kubernetes.Interface, absCtx absconfig.ABSContext, ns string) error {
	se, err := kubecli.CoreV1().Secrets(ns).Get(absCtx.ABSSecret, metav1.GetOptions{})
	if err != nil {
		return err
	}

	os.Setenv(env.ABSStorageAccount, string(se.Data[spec.ABSStorageAccount]))
	os.Setenv(env.ABSStorageKey, string(se.Data[spec.ABSStorageKey]))

	return nil
}
