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
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/client-go/kubernetes"
)

type pv struct {
	clusterName  string
	namespace    string
	storageClass string
	backupPolicy api.BackupPolicy
	kubecli      kubernetes.Interface
}

func NewPVStorage(kubecli kubernetes.Interface, cn, ns, sc string, backupPolicy api.BackupPolicy) (Storage, error) {
	s := &pv{
		clusterName:  cn,
		namespace:    ns,
		storageClass: sc,
		backupPolicy: backupPolicy,
		kubecli:      kubecli,
	}
	return s, nil
}

func (s *pv) Create() error {
	return k8sutil.CreateAndWaitPVC(s.kubecli, s.clusterName, s.namespace, s.storageClass, s.backupPolicy.PV.VolumeSizeInMB)
}

func (s *pv) Clone(from string) error {
	return k8sutil.CopyVolume(s.kubecli, from, s.clusterName, s.namespace)
}

func (s *pv) Delete() error {
	if s.backupPolicy.AutoDelete {
		return k8sutil.DeletePVC(s.kubecli, s.clusterName, s.namespace)
	}
	return nil
}
