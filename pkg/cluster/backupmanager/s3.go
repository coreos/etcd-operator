package backupmanager

import (
	"io/ioutil"
	"os"

	"github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type s3BackupManager struct {
	s3config.S3Context
	clusterName string
	namespace   string
	kubecli     *unversioned.Client
	s3cli       *s3.S3
}

func NewS3BackupManager(s3Ctx s3config.S3Context, kubecli *unversioned.Client, clusterName, ns string) (BackupManager, error) {
	cm, err := kubecli.ConfigMaps(ns).Get(s3Ctx.AWSConfig)
	if err != nil {
		return nil, err
	}
	se, err := kubecli.Secrets(ns).Get(s3Ctx.AWSSecret)
	if err != nil {
		return nil, err
	}
	err = setupS3Env([]byte(cm.Data["config"]), se.Data["credentials"])
	if err != nil {
		return nil, err
	}

	s3cli, err := s3.New(s3Ctx.S3Bucket, clusterName+"/")
	if err != nil {
		return nil, err
	}

	bm := &s3BackupManager{
		S3Context:   s3Ctx,
		kubecli:     kubecli,
		clusterName: clusterName,
		namespace:   ns,
		s3cli:       s3cli,
	}
	return bm, nil
}

func setupS3Env(config, creds []byte) error {
	homedir := os.Getenv("HOME")
	if err := os.MkdirAll(homedir+"/.aws", 0700); err != nil {
		return err
	}
	if err := ioutil.WriteFile(homedir+"/.aws/config", config, 0600); err != nil {
		return err
	}
	return ioutil.WriteFile(homedir+"/.aws/credentials", creds, 0600)
}

func (bm *s3BackupManager) Setup() error {
	// TODO: check if bucket/folder exists?
	return nil
}

func (bm *s3BackupManager) Clone(from string) error {
	return bm.s3cli.CopyPrefix(from)
}

func (bm *s3BackupManager) CleanupBackups() error {
	// TODO: remove S3 "dir"?
	return nil
}
func (bm *s3BackupManager) PodSpecWithStorage(ps *api.PodSpec) *api.PodSpec {
	return k8sutil.PodSpecWithS3(ps, bm.S3Context)
}
