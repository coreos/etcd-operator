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

package backupstorage

import (
	"path"

	backupabs "github.com/coreos/etcd-operator/pkg/backup/abs"
	"github.com/coreos/etcd-operator/pkg/spec"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type abs struct {
	clusterName  string
	namespace    string
	backupPolicy spec.BackupPolicy
	kubecli      kubernetes.Interface
	abscli       *backupabs.ABS
}

// NewABSStorage returns a new ABS Storage implementation using the given kubecli, cluster name, namespace and backup policy
func NewABSStorage(kubecli kubernetes.Interface, clusterName, ns string, p spec.BackupPolicy) (Storage, error) {
	prefix := path.Join(ns, clusterName)

	abscli, err := func() (*backupabs.ABS, error) {
		account, key, err := setupABSCreds(kubecli, ns, p.ABS.ABSSecret)
		if err != nil {
			return nil, err
		}
		return backupabs.New(p.ABS.ABSContainer, account, key, prefix)
	}()
	if err != nil {
		return nil, err
	}

	ws := &abs{
		kubecli:      kubecli,
		clusterName:  clusterName,
		backupPolicy: p,
		namespace:    ns,
		abscli:       abscli,
	}
	return ws, nil
}

func (a *abs) Create() error {
	// TODO: check if container exists?
	return nil
}

func (a *abs) Clone(from string) error {
	prefix := a.namespace + "/" + from
	return a.abscli.CopyPrefix(prefix)
}

func (a *abs) Delete() error {
	if a.backupPolicy.AutoDelete {
		names, err := a.abscli.List()
		if err != nil {
			return err
		}
		for _, n := range names {
			err = a.abscli.Delete(n)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func setupABSCreds(kubecli kubernetes.Interface, ns, secret string) (account, key string, err error) {
	se, err := kubecli.CoreV1().Secrets(ns).Get(secret, metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	account = string(se.Data[spec.ABSStorageAccount])
	key = string(se.Data[spec.ABSStorageKey])
	return
}
