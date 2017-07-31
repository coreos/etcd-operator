package backupstorage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	backups3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"

	"github.com/aws/aws-sdk-go/aws/session"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type s3 struct {
	s3config.S3Context
	clusterName  string
	namespace    string
	credsDir     string
	backupPolicy spec.BackupPolicy
	kubecli      kubernetes.Interface
	s3cli        *backups3.S3
}

func NewS3Storage(s3Ctx s3config.S3Context, kubecli kubernetes.Interface, clusterName, ns string, p spec.BackupPolicy) (Storage, error) {
	prefix := path.Join(ns, clusterName)

	var dir string

	s3cli, err := func() (*backups3.S3, error) {
		if p.S3 != nil {
			dir = filepath.Join(constants.OperatorRoot, "aws", prefix)
			if err := os.MkdirAll(dir, 0700); err != nil {
				return nil, err
			}
			options, err := setupAWSConfig(kubecli, ns, p.S3.AWSSecret, dir)
			if err != nil {
				return nil, err
			}
			return backups3.NewFromSessionOpt(p.S3.S3Bucket, p.S3.S3Path, prefix, *options)
		} else {
			return backups3.New(s3Ctx.S3Bucket, p.S3.S3Path, prefix)
		}
	}()
	if err != nil {
		return nil, fmt.Errorf("new S3 storage failed: %v", err)
	}

	s := &s3{
		S3Context:    s3Ctx,
		kubecli:      kubecli,
		clusterName:  clusterName,
		backupPolicy: p,
		namespace:    ns,
		s3cli:        s3cli,
		credsDir:     dir,
	}
	return s, nil
}

func (s *s3) Create() error {
	// TODO: check if bucket/folder exists?
	return nil
}

func (s *s3) Clone(from string) error {
	prefix := s.namespace + "/" + from
	return s.s3cli.CopyPrefix(prefix)
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

	creds := se.Data[spec.AWSSecretCredentialsFileName]
	if len(creds) != 0 {
		credsFile := path.Join(dir, "credentials")
		err = ioutil.WriteFile(credsFile, creds, 0600)
		if err != nil {
			return nil, fmt.Errorf("setup AWS config failed: write credentials file failed: %v", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, credsFile)
	}

	config := se.Data[spec.AWSSecretConfigFileName]
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
