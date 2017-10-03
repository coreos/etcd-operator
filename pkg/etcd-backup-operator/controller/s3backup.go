// Copyright 2017 The etcd-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"path"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/coreos/etcd/clientv3"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	S3V1                 = "v1"
	backupFilenameSuffix = "etcd.backup"
	tmpDir               = "/tmp"
)

type s3Backup struct {
	namespace   string
	clusterName string

	s3bucket  string
	prefix    string
	awsSecret string

	kubecli kubernetes.Interface
}

func (s *s3Backup) saveSnap() error {
	podList, err := s.kubecli.Core().Pods(s.namespace).List(k8sutil.ClusterListOpt(s.clusterName))
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
		msg := "no reachable member"
		logrus.Warning(msg)
		return fmt.Errorf(msg)
	}

	logrus.Infof("saving backup for cluster %s ...", s.clusterName)
	if err := s.writeSnap(member, rev); err != nil {
		msg := fmt.Sprintf("failed to save backup for cluster %s: (%v)", s.clusterName, err)
		logrus.Warning(msg)
		return fmt.Errorf(msg)
	}
	logrus.Infof("saving backup for cluster %s succeeded", s.clusterName)
	return nil
}

func (s *s3Backup) writeSnap(m *etcdutil.Member, rev int64) error {
	cfg := clientv3.Config{
		Endpoints:   []string{m.ClientURL()},
		DialTimeout: constants.DefaultDialTimeout,
		TLS:         nil,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create etcd client: (%v)", err)
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.Maintenance.Status(ctx, m.ClientURL())
	cancel()
	if err != nil {
		return fmt.Errorf("failed to get member %v status: (%v)", m.ClientURL(), err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), constants.DefaultSnapshotTimeout)
	defer cancel()
	rc, err := etcdcli.Maintenance.Snapshot(ctx)
	defer rc.Close()
	if err != nil {
		return fmt.Errorf("failed to receive snapshot: (%v)", err)
	}
	err = s.saveToS3(resp.Version, rev, rc)
	if err != nil {
		return fmt.Errorf("failed to save snapshot: (%v)", err)
	}
	return nil
}

func (s *s3Backup) saveToS3(version string, rev int64, rc io.ReadCloser) error {
	secret, err := s.kubecli.Core().Secrets(s.namespace).Get(s.awsSecret, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to retrieve secret %v: (%v)", s.awsSecret, err)
	}
	s3u, err := newS3Uploader(s.s3bucket, s.prefix, secret)
	if err != nil {
		return fmt.Errorf("failed to create s3 uploader: (%v)", err)
	}
	key := path.Join(s.prefix, makeBackupName(version, rev))
	ui := &s3manager.UploadInput{
		Bucket: aws.String(s.s3bucket),
		Key:    aws.String(key),
		Body:   rc,
	}
	_, err = s3u.Upload(ui)
	if err != nil {
		return fmt.Errorf("failed to upload snapshot to s3: (%v)", err)
	}
	return nil
}

func makeBackupName(ver string, rev int64) string {
	return fmt.Sprintf("%s_%016x_%s", ver, rev, backupFilenameSuffix)
}

// NewS3Uploader returns a Uploader that is configured with proper aws config and credentials derived from Kubernetes secret.
func newS3Uploader(bucket, prefix string, secret *v1.Secret) (*s3manager.Uploader, error) {
	so, err := newAWSConfig(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to setup aws config: (%v)", err)
	}
	sess, err := session.NewSessionWithOptions(*so)
	if err != nil {
		return nil, fmt.Errorf("failed to create new AWS session: (%v)", err)
	}
	return s3manager.NewUploader(sess), nil
}

func newAWSConfig(secret *v1.Secret) (*session.Options, error) {
	dir, err := ioutil.TempDir(tmpDir, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create dir %v: (%v)", dir, err)
	}

	options := &session.Options{}
	options.SharedConfigState = session.SharedConfigEnable

	creds := secret.Data[api.AWSSecretCredentialsFileName]
	if len(creds) != 0 {
		credsFile := path.Join(dir, "credentials")
		err := ioutil.WriteFile(credsFile, creds, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to write credentials file: (%v)", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, credsFile)
	}

	config := secret.Data[api.AWSSecretConfigFileName]
	if config != nil {
		configFile := path.Join(dir, "config")
		err := ioutil.WriteFile(configFile, config, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to write config file: (%v)", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, configFile)
	}

	return options, nil
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
			logrus.Warningf("failed to create etcd client for pod %v: (%v)", pod.Name, err)
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

// toS3Prefix concatenates s3Prefix, S3V1, namespace, clusterName to a single s3 prefix.
// the concatenated prefix determines the location of S3 backup files.
func toS3Prefix(s3Prefix, namespace, clusterName string) string {
	return path.Join(s3Prefix, S3V1, namespace, clusterName)
}
