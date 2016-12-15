package backupmanager

import (
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type pvBackupManager struct {
	clusterName   string
	namespace     string
	pvProvisioner string
	backupPolicy  spec.BackupPolicy
	kubecli       *unversioned.Client
}

func NewPVBackupManager(kc *unversioned.Client, cn, ns, pvp string, backupPolicy spec.BackupPolicy) BackupManager {
	return &pvBackupManager{
		clusterName:   cn,
		namespace:     ns,
		pvProvisioner: pvp,
		backupPolicy:  backupPolicy,
		kubecli:       kc,
	}
}

func (bm *pvBackupManager) Setup() error {
	return k8sutil.CreateAndWaitPVC(bm.kubecli, bm.clusterName, bm.namespace, bm.pvProvisioner, bm.backupPolicy.VolumeSizeInMB)
}

func (bm *pvBackupManager) Clone(from string) error {
	err := k8sutil.CreateAndWaitPVC(bm.kubecli, bm.clusterName, bm.namespace, bm.pvProvisioner, bm.backupPolicy.VolumeSizeInMB)
	if err != nil {
		return err
	}
	return k8sutil.CopyVolume(bm.kubecli, from, bm.clusterName, bm.namespace)
}

func (bm *pvBackupManager) CleanupBackups() error {
	return k8sutil.DeleteBackupReplicaSetAndService(bm.kubecli, bm.clusterName, bm.namespace, bm.backupPolicy.CleanupBackupsOnClusterDelete)
}

func (bm *pvBackupManager) PodSpecWithStorage(ps *api.PodSpec) *api.PodSpec {
	return k8sutil.PodSpecWithPV(ps, bm.clusterName)
}
