package backupstorage

import (
	"os"
	"path"

	"github.com/aws/aws-sdk-go/aws/credentials"

	"github.com/aws/aws-sdk-go/aws"
	backups3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/spec"

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
			opts := session.Options{}
			if p.S3.AWSSecret != "" {
				creds, region, err := setupAWSConfig(kubecli, ns, p.S3.AWSSecret, dir)
				if err != nil {
					return nil, err
				}

				opts.Config = aws.Config{
					Credentials: credentials.NewStaticCredentialsFromCreds(*creds),
					Region:      region,
				}
			}

			return backups3.NewFromSessionOpt(p.S3.S3Bucket, prefix, opts)
		}
		return backups3.New(s3Ctx.S3Bucket, prefix)
	}()
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
	if s.backupPolicy.S3 != nil {
		return os.RemoveAll(s.credsDir)
	}
	return nil
}

func setupAWSConfig(kubecli kubernetes.Interface, ns, secret, dir string) (*credentials.Value, *string, error) {
	se, err := kubecli.CoreV1().Secrets(ns).Get(secret, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	credsData := se.Data[spec.AWSSecretCredentialsFileName]
	creds, err := backups3.LoadCredentials(credsData)
	if err != nil {
		return nil, nil, err
	}

	configData := se.Data[spec.AWSSecretConfigFileName]
	region, err := backups3.LoadConfig(configData)
	if err != nil {
		return nil, nil, err
	}

	return creds, &region, nil
}
