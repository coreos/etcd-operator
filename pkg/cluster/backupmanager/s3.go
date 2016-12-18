package backupmanager

import (
	"fmt"

	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type s3BackupManager struct {
	s3config.S3Context
	clusterName  string
	namespace    string
	backupPolicy spec.BackupPolicy
	kubecli      *unversioned.Client
}

func NewS3BackupManager(s3Ctx s3config.S3Context, kc *unversioned.Client, cn, ns string, backupPolicy spec.BackupPolicy) BackupManager {
	return &s3BackupManager{
		S3Context:    s3Ctx,
		clusterName:  cn,
		namespace:    ns,
		backupPolicy: backupPolicy,
		kubecli:      kc,
	}
}

func (bm *s3BackupManager) Setup() error {
	// TODO: check if bucket/folder exists?
	return nil
}
func (bm *s3BackupManager) Clone(from string) error {
	return fmt.Errorf("TODO: unsupported Cloning previous S3 data")
}
func (bm *s3BackupManager) Cleanup() error {
	err := k8sutil.DeleteBackupReplicaSetAndService(bm.kubecli, bm.clusterName, bm.namespace)
	if err != nil {
		return err
	}
	if bm.backupPolicy.CleanupBackupsOnClusterDelete {
		// TODO: remove S3 "dir"?
	}
	return nil
}
func (bm *s3BackupManager) PodSpecWithStorage(ps *api.PodSpec) *api.PodSpec {
	return k8sutil.PodSpecWithS3(ps, bm.S3Context)
}
