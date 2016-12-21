package backupstorage

import (
	"io/ioutil"
	"os"

	backups3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"

	"github.com/aws/aws-sdk-go/aws/session"
	"k8s.io/kubernetes/pkg/client/unversioned"
)

type s3 struct {
	s3config.S3Context
	clusterName string
	namespace   string
	kubecli     *unversioned.Client
	s3cli       *backups3.S3
}

func NewS3Storage(s3Ctx s3config.S3Context, kubecli *unversioned.Client, clusterName, ns string) (Storage, error) {
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

	s3cli, err := backups3.New(s3Ctx.S3Bucket, clusterName+"/", session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, err
	}

	s := &s3{
		S3Context:   s3Ctx,
		kubecli:     kubecli,
		clusterName: clusterName,
		namespace:   ns,
		s3cli:       s3cli,
	}
	if err := s.setup(); err != nil {
		return nil, err
	}
	return s, nil
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

func (s *s3) setup() error {
	// TODO: check if bucket/folder exists?
	return nil
}

func (s *s3) Clone(from string) error {
	return s.s3cli.CopyPrefix(from)
}

func (s *s3) Delete() error {
	// TODO: remove S3 "dir"?
	return nil
}
