package backupstorage

import (
	backups3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/spec"

	"github.com/aws/aws-sdk-go/aws/session"
	"k8s.io/client-go/kubernetes"
)

type s3 struct {
	s3config.S3Context
	clusterName  string
	namespace    string
	backupPolicy spec.BackupPolicy
	kubecli      kubernetes.Interface
	s3cli        *backups3.S3
}

func NewS3Storage(s3Ctx s3config.S3Context, kubecli kubernetes.Interface, clusterName, ns string, p spec.BackupPolicy) (Storage, error) {
	s3cli, err := backups3.New(s3Ctx.S3Bucket, clusterName, session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, err
	}

	s := &s3{
		S3Context:    s3Ctx,
		kubecli:      kubecli,
		clusterName:  clusterName,
		backupPolicy: p,
		namespace:    ns,
		s3cli:        s3cli,
	}
	return s, nil
}

func (s *s3) Create() error {
	// TODO: check if bucket/folder exists?
	return nil
}

func (s *s3) Clone(from string) error {
	return s.s3cli.CopyPrefix(from)
}

func (s *s3) Delete() error {
	if s.backupPolicy.CleanupBackupsOnClusterDelete {
		names, err := s.s3cli.List()
		if err != nil {
			return err
		}
		for _, n := range names {
			err = s.s3cli.Delete(n)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
