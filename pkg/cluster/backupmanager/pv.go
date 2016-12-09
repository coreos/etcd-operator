package backupmanager

import (
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type pvBackupManager struct {
	clusterName    string
	namespace      string
	pvProvisioner  string
	volumeSizeInMB int
	kubecli        *unversioned.Client
}

func NewPVBackupManager(kc *unversioned.Client, cn, ns, pvp string, vs int) BackupManager {
	return &pvBackupManager{
		clusterName:    cn,
		namespace:      ns,
		pvProvisioner:  pvp,
		volumeSizeInMB: vs,
		kubecli:        kc,
	}
}

func (bm *pvBackupManager) Setup() error {
	return k8sutil.CreateAndWaitPVC(bm.kubecli, bm.clusterName, bm.namespace, bm.pvProvisioner, bm.volumeSizeInMB)
}

func (bm *pvBackupManager) Clone(from string) error {
	err := k8sutil.CreateAndWaitPVC(bm.kubecli, bm.clusterName, bm.namespace, bm.pvProvisioner, bm.volumeSizeInMB)
	if err != nil {
		return err
	}
	return k8sutil.CopyVolume(bm.kubecli, from, bm.clusterName, bm.namespace)
}

func (bm *pvBackupManager) PodSpecWithStorage(ps *api.PodSpec) *api.PodSpec {
	return k8sutil.PodSpecWithPV(ps, bm.clusterName)
}
