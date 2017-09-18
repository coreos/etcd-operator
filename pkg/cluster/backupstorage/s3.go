package backupstorage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta1"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	backups3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/util/constants"

	"github.com/aws/aws-sdk-go/aws/session"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type s3 struct {
	clusterName  string
	namespace    string
	s3Prefix     string
	credsDir     string
	backupPolicy api.BackupPolicy
	kubecli      kubernetes.Interface
	s3cli        *backups3.S3
}

func NewS3Storage(kubecli kubernetes.Interface, clusterName, ns string, p api.BackupPolicy) (Storage, error) {
	var (
		prefix = backupapi.ToS3Prefix(p.S3.Prefix, ns, clusterName)
		dir    string
	)
	s3cli, err := func() (*backups3.S3, error) {
		dir = filepath.Join(constants.OperatorRoot, "aws", prefix)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		options, err := setupAWSConfig(kubecli, ns, p.S3.AWSSecret, dir)
		if err != nil {
			return nil, err
		}
		return backups3.NewFromSessionOpt(p.S3.S3Bucket, prefix, *options)
	}()
	if err != nil {
		return nil, fmt.Errorf("new S3 storage failed: %v", err)
	}

	s := &s3{
		kubecli:      kubecli,
		clusterName:  clusterName,
		backupPolicy: p,
		namespace:    ns,
		s3Prefix:     p.S3.Prefix,
		s3cli:        s3cli,
		credsDir:     dir,
	}
	return s, nil
}

func (s *s3) Create() error {
	// TODO: check if bucket/folder exists?
	return nil
}

func (s *s3) Clone(fromCluster string) error {
	return s.s3cli.CopyPrefix(backupapi.ToS3Prefix(s.s3Prefix, s.namespace, fromCluster))
}

func (s *s3) Delete() error {
	if s.backupPolicy.AutoDelete {
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
	if s.backupPolicy.S3 != nil {
		return os.RemoveAll(s.credsDir)
	}
	return nil
}

func setupAWSConfig(kubecli kubernetes.Interface, ns, secret, dir string) (*session.Options, error) {
	options := &session.Options{}
	options.SharedConfigState = session.SharedConfigEnable

	se, err := kubecli.CoreV1().Secrets(ns).Get(secret, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("setup AWS config failed: get k8s secret failed: %v", err)
	}

	creds := se.Data[api.AWSSecretCredentialsFileName]
	if len(creds) != 0 {
		credsFile := path.Join(dir, "credentials")
		err = ioutil.WriteFile(credsFile, creds, 0600)
		if err != nil {
			return nil, fmt.Errorf("setup AWS config failed: write credentials file failed: %v", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, credsFile)
	}

	config := se.Data[api.AWSSecretConfigFileName]
	if config != nil {
		configFile := path.Join(dir, "config")
		err = ioutil.WriteFile(configFile, config, 0600)
		if err != nil {
			return nil, fmt.Errorf("setup AWS config failed: write config file failed: %v", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, configFile)
	}

	return options, nil
}
