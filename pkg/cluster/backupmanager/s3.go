package backupmanager

import (
	"fmt"

	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/kubernetes/pkg/api"
)

type s3BackupManager struct {
	s3config.S3Context
}

func NewS3BackupManager(s3Ctx s3config.S3Context) BackupManager {
	return &s3BackupManager{
		S3Context: s3Ctx,
	}
}

func (bm *s3BackupManager) Setup() error {
	// TODO: check if bucket/folder exists?
	return nil
}
func (bm *s3BackupManager) Clone(from string) error {
	return fmt.Errorf("TODO: unsupported Cloning previous S3 data")
}
func (bm *s3BackupManager) PodSpecWithStorage(ps *api.PodSpec) *api.PodSpec {
	return k8sutil.PodSpecWithS3(ps, bm.S3Context)
}
