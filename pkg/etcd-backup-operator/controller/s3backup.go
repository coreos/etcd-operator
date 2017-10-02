package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	apis "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd/clientv3"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (b *Backup) handleS3(clusterName string, s3 *apis.S3Source) error {
	s3b := &s3Backup{
		namespace:   b.namespace,
		name:        b.name,
		clusterName: clusterName,
		s3bucket:    s3.S3Bucket,
		prefix:      ToS3Prefix(s3.Prefix, b.namespace, clusterName),
		awsSecret:   s3.AWSSecret,
		kubecli:     b.kubecli,
	}
	return s3b.saveSnap()
}

const S3V1 = "v1"

// ToS3Prefix concatenates s3Prefix, S3V1, namespace, clusterName to a single s3 prefix.
// the concatenated prefix determines the location of S3 backup files.
func ToS3Prefix(s3Prefix, namespace, clusterName string) string {
	return path.Join(s3Prefix, S3V1, namespace, clusterName)
}

type s3Backup struct {
	namespace   string
	name        string
	clusterName string

	s3bucket  string
	prefix    string
	awsSecret string

	kubecli kubernetes.Interface
}

func (b *s3Backup) saveSnap() error {
	podList, err := b.kubecli.Core().Pods(b.namespace).List(k8sutil.ClusterListOpt(b.clusterName))
	if err != nil {
		return err
	}
	var pods []*v1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == v1.PodRunning {
			pods = append(pods, pod)
		}
	}

	if len(pods) == 0 {
		msg := "no running etcd pods found"
		logrus.Warning(msg)
		return fmt.Errorf(msg)
	}
	member, rev := getMemberWithMaxRev(pods, nil)
	if member == nil {
		logrus.Warning("no reachable member")
		return fmt.Errorf("no reachable member")
	}

	log.Printf("saving backup for cluster (%s)", b.clusterName)
	if err := b.writeSnap(member, rev); err != nil {
		err = fmt.Errorf("write snapshot failed: %v", err)
		return err
	}
	return nil
}

func (b *s3Backup) writeSnap(m *etcdutil.Member, rev int64) error {
	cfg := clientv3.Config{
		Endpoints:   []string{m.ClientURL()},
		DialTimeout: constants.DefaultDialTimeout,
		TLS:         nil,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create etcd client (%v)", err)
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.Maintenance.Status(ctx, m.ClientURL())
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultSnapshotTimeout)
	defer cancel()
	rc, err := etcdcli.Maintenance.Snapshot(ctx)
	defer rc.Close()
	if err != nil {
		return fmt.Errorf("failed to receive snapshot (%v)", err)
	}
	return b.saveToS3(resp.Version, rev, rc)
}

// NewS3Uploader returns a Uploader that is configured with proper aws config and credentials derived from Kubernetes secret.
func NewS3Uploader(bucket, prefix string, secret *v1.Secret) (*s3manager.Uploader, error) {
	so, err := newAWSConfig(secret, prefix)
	if err != nil {
		return nil, err
	}
	return NewFromSessionOpt(bucket, prefix, *so)
}

const operatorRoot = "/var/tmp/etcd-backup-operator"

func newAWSConfig(secret *v1.Secret, prefix string) (*session.Options, error) {
	dir := filepath.Join(operatorRoot, "aws", prefix)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	options := &session.Options{}
	options.SharedConfigState = session.SharedConfigEnable

	creds := secret.Data[apis.AWSSecretCredentialsFileName]
	if len(creds) != 0 {
		credsFile := path.Join(dir, "credentials")
		err := ioutil.WriteFile(credsFile, creds, 0600)
		if err != nil {
			return nil, fmt.Errorf("setup AWS config failed: write credentials file failed: %v", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, credsFile)
	}

	config := secret.Data[apis.AWSSecretConfigFileName]
	if config != nil {
		configFile := path.Join(dir, "config")
		err := ioutil.WriteFile(configFile, config, 0600)
		if err != nil {
			return nil, fmt.Errorf("setup AWS config failed: write config file failed: %v", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, configFile)
	}

	return options, nil
}

func NewFromSessionOpt(bucket, prefix string, so session.Options) (*s3manager.Uploader, error) {
	sess, err := session.NewSessionWithOptions(so)
	if err != nil {
		return nil, fmt.Errorf("new AWS session failed: %v", err)
	}
	return s3manager.NewUploader(sess), nil
}

func (b *s3Backup) saveToS3(version string, rev int64, rc io.ReadCloser) error {
	secret, err := b.kubecli.Core().Secrets(b.namespace).Get(b.awsSecret, metav1.GetOptions{})
	if err != nil {
		return err
	}
	s3u, err := NewS3Uploader(b.s3bucket, b.prefix, secret)
	if err != nil {
		return err
	}
	key := path.Join(b.prefix, makeBackupName(version, rev))
	ui := &s3manager.UploadInput{
		Bucket: aws.String(b.s3bucket),
		Key:    aws.String(key),
		Body:   rc,
	}
	_, err = s3u.Upload(ui)
	return err
}

const backupFilenameSuffix = "etcd.backup"

func makeBackupName(ver string, rev int64) string {
	return fmt.Sprintf("%s_%016x_%s", ver, rev, backupFilenameSuffix)
}

func getMemberWithMaxRev(pods []*v1.Pod, tc *tls.Config) (*etcdutil.Member, int64) {
	var member *etcdutil.Member
	maxRev := int64(0)
	for _, pod := range pods {
		m := &etcdutil.Member{
			Name:         pod.Name,
			Namespace:    pod.Namespace,
			SecureClient: tc != nil,
		}
		cfg := clientv3.Config{
			Endpoints:   []string{m.ClientURL()},
			DialTimeout: constants.DefaultDialTimeout,
			TLS:         tc,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			logrus.Warningf("failed to create etcd client for pod (%v): %v", pod.Name, err)
			continue
		}
		defer etcdcli.Close()
		ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
		resp, err := etcdcli.Get(ctx, "/", clientv3.WithSerializable())
		cancel()
		if err != nil {
			logrus.Warningf("getMaxRev: failed to get revision from member %s (%s)", m.Name, m.ClientURL())
			continue
		}
		logrus.Infof("getMaxRev: member %s revision (%d)", m.Name, resp.Header.Revision)
		if resp.Header.Revision > maxRev {
			maxRev = resp.Header.Revision
			member = m
		}
	}
	return member, maxRev
}
