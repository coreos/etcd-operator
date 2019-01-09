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

package s3factory

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	tmpdir = "/tmp"
)

// S3Client is a wrapper for S3 client that provides cleanup functionality.
type S3Client struct {
	S3        *s3.S3
	configDir string
}

// NewClientFromSecret returns a S3 client based on given k8s secret containing aws credentials.
func NewClientFromSecret(kubecli kubernetes.Interface, namespace, endpoint, awsSecret string, forcePathStyle bool) (w *S3Client, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("new S3 client failed: %v", err)
		}
	}()
	w = &S3Client{}
	w.configDir, err = ioutil.TempDir(tmpdir, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create aws config dir: (%v)", err)
	}
	so, err := setupAWSConfig(kubecli, namespace, awsSecret, endpoint, w.configDir, forcePathStyle)
	if err != nil {
		return nil, fmt.Errorf("failed to setup aws config: (%v)", err)
	}
	sess, err := session.NewSessionWithOptions(*so)
	if err != nil {
		return nil, fmt.Errorf("new AWS session failed: %v", err)
	}
	w.S3 = s3.New(sess)
	return w, nil
}

// Close cleans up all intermediate resources for creating S3 client.
func (w *S3Client) Close() {
	os.RemoveAll(w.configDir)
}

// setupAWSConfig setup local AWS config/credential files from Kubernetes aws secret.
func setupAWSConfig(kubecli kubernetes.Interface, ns, secret, endpoint, configDir string, forcePathStyle bool) (*session.Options, error) {
	options := &session.Options{}
	options.SharedConfigState = session.SharedConfigEnable

	// empty string defaults to aws
	options.Config.Endpoint = &endpoint

	options.Config.S3ForcePathStyle = &forcePathStyle

	se, err := kubecli.CoreV1().Secrets(ns).Get(secret, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("setup AWS config failed: get k8s secret failed: %v", err)
	}

	creds := se.Data[api.AWSSecretCredentialsFileName]
	if len(creds) != 0 {
		credsFile := path.Join(configDir, "credentials")
		err = ioutil.WriteFile(credsFile, creds, 0600)
		if err != nil {
			return nil, fmt.Errorf("setup AWS config failed: write credentials file failed: %v", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, credsFile)
	}

	config := se.Data[api.AWSSecretConfigFileName]
	if len(config) != 0 {
		configFile := path.Join(configDir, "config")
		err = ioutil.WriteFile(configFile, config, 0600)
		if err != nil {
			return nil, fmt.Errorf("setup AWS config failed: write config file failed: %v", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, configFile)
	}

	return options, nil
}
