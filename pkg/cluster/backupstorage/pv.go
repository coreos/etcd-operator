package backupstorage

import (
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/kubernetes/pkg/client/unversioned"
)

type pv struct {
	clusterName   string
	namespace     string
	pvProvisioner string
	backupPolicy  spec.BackupPolicy
	kubecli       *unversioned.Client
}

func NewPVStorage(kc *unversioned.Client, cn, ns, pvp string, backupPolicy spec.BackupPolicy) (Storage, error) {
	s := &pv{
		clusterName:   cn,
		namespace:     ns,
		pvProvisioner: pvp,
		backupPolicy:  backupPolicy,
		kubecli:       kc,
	}
	return s, nil
}

func (s *pv) Create() error {
	return k8sutil.CreateAndWaitPVC(s.kubecli, s.clusterName, s.namespace, s.pvProvisioner, s.backupPolicy.VolumeSizeInMB)
}

func (s *pv) Clone(from string) error {
	return k8sutil.CopyVolume(s.kubecli, from, s.clusterName, s.namespace)
}

func (s *pv) Delete() error {
	if s.backupPolicy.CleanupBackupsOnClusterDelete {
		return k8sutil.DeletePVC(s.kubecli, s.clusterName, s.namespace)
	}
	return nil
}
