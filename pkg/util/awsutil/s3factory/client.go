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
	"net/url"
	"os"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/minio/minio-go/pkg/s3utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
)

const (
	tmpdir = "/tmp"
)

// S3Client is a wrapper for S3 client that provides cleanup functionality.
type S3Client struct {
	S3        *s3.S3
	configDir string
}

// ClientConfig is the configuration for a S3 client.
type ClientConfig struct {
	Endpoint  string
	Namespace string
	AWSSecret string
}

// NewClient returns a new S3Client. This client can be based on profiles, keys...
// (from secret), or IAM Role based (nodes, Kube2iam, Kiam...), the client
// type will be infered based on the missing (or not missing) data in the
// client configuration.
func NewClient(cfg ClientConfig, kubecli kubernetes.Interface) (*S3Client, error) {
	// If credentials not required then use the S3 client based on IAM roles.
	if cfg.AWSSecret == "" {
		return newClientForIAMRole(cfg.Endpoint)
	}

	return newClientFromSecret(kubecli, cfg.Namespace, cfg.Endpoint, cfg.AWSSecret)
}

// newClientForIAMRole returns a S3 client that doesn't have credentials set and will do that
// AWS S3 client fallback to machines assigned IAM role (or in case of using Kube2iam, Kiam...,
// use the methods used to act on befhalf of the assumed roles given by these systems).
func newClientForIAMRole(endpoint string) (w *S3Client, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("new S3 client failed: %v", err)
		}
	}()

	// Get the region based on the endpoint.
	region := s3utils.GetRegionFromURL(url.URL{
		Host: endpoint,
	})

	if region == "" {
		return nil, fmt.Errorf("could not infer region from '%s' s3 endpoint, use an endpoint with region like `s3.us-east-1.amazonaws.com`", endpoint)
	}

	// Create AWS session default the config files, profile...
	sess, err := session.NewSession(&aws.Config{
		Endpoint: aws.String(endpoint),
		Region:   aws.String(region),
	})
	if err != nil {
		return nil, err
	}

	return &S3Client{
		S3: s3.New(sess),
	}, nil
}

// newClientFromSecret returns a S3 client based on given k8s secret containing aws credentials.
func newClientFromSecret(kubecli kubernetes.Interface, namespace, endpoint, awsSecret string) (w *S3Client, err error) {
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
	so, err := setupAWSConfig(kubecli, namespace, awsSecret, endpoint, w.configDir)
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
func setupAWSConfig(kubecli kubernetes.Interface, ns, secret, endpoint, configDir string) (*session.Options, error) {
	options := &session.Options{}
	options.SharedConfigState = session.SharedConfigEnable

	// empty string defaults to aws
	options.Config.Endpoint = aws.String(endpoint)

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
