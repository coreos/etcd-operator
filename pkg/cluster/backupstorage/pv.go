package backupstorage

import (
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/client-go/kubernetes"
)

type pv struct {
	clusterName  string
	namespace    string
	storageClass string
	backupPolicy spec.BackupPolicy
	kubecli      kubernetes.Interface
}

func NewPVStorage(kubecli kubernetes.Interface, cn, ns, sc string, backupPolicy spec.BackupPolicy) (Storage, error) {
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
