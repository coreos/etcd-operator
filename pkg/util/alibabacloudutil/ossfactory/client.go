// Copyright 2019 The etcd-operator Authors
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

package ossfactory

import (
	"fmt"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// OSSClient is a wrapper of OSS client that provides cleanup functionality.
type OSSClient struct {
	OSS *oss.Client
}

// NewClientFromSecret returns a OSS client based on given k8s secret containing alibabacloud credentials.
func NewClientFromSecret(kubecli kubernetes.Interface, namespace, endpoint, ossSecret string) (w *OSSClient, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("new OSS client failed: %v", err)
		}
	}()

	se, err := kubecli.CoreV1().Secrets(namespace).Get(ossSecret, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get k8s secret: %v", err)
	}

	accessKeyID, ok := se.Data[api.AlibabaCloudSecretCredentialsAccessKeyID]
	if !ok {
		return nil, fmt.Errorf("key \"%s\" not found in secret \"%s\" in namespace \"%s\"",
			api.AlibabaCloudSecretCredentialsAccessKeyID, ossSecret, namespace)
	}

	accessKeySecret, ok := se.Data[api.AlibabaCloudSecretCredentialsAccessKeySecret]
	if !ok {
		return nil, fmt.Errorf("key \"%s\" not found in secret \"%s\" in namespace \"%s\"",
			api.AlibabaCloudSecretCredentialsAccessKeySecret, ossSecret, namespace)
	}

	client, err := oss.New(endpoint, string(accessKeyID), string(accessKeySecret))
	if err != nil {
		return nil, fmt.Errorf("failed to create OSS client: %v", err)
	}
	return &OSSClient{OSS: client}, nil
}
