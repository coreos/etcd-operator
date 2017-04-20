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
	"io/ioutil"
	"os"
	"path"

	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func setupS3Env(kubecli kubernetes.Interface, s3Ctx s3config.S3Context, ns string) error {
	cm, err := kubecli.CoreV1().ConfigMaps(ns).Get(s3Ctx.AWSConfig, metav1.GetOptions{})
	if err != nil {
		return err
	}
	se, err := kubecli.CoreV1().Secrets(ns).Get(s3Ctx.AWSSecret, metav1.GetOptions{})
	if err != nil {
		return err
	}

	config := []byte(cm.Data["config"])
	creds := se.Data["credentials"]

	homedir := os.Getenv("HOME")
	if err := os.MkdirAll(path.Join(homedir, "/.aws"), 0700); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(homedir, "/.aws/config"), config, 0600); err != nil {
		return err
	}
	return ioutil.WriteFile(path.Join(homedir, "/.aws/credentials"), creds, 0600)
}
