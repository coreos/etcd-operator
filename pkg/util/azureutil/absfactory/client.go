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

package absfactory

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/Azure/go-autorest/autorest/azure"
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ABSClient is a wrapper of ABS client that provides cleanup functionality.
type ABSClient struct {
	ABS *storage.BlobStorageClient
}

// parseAzureEnvironment returns azure environment by name
func parseAzureEnvironment(cloudName string) (azure.Environment, error) {
	if cloudName == "" {
		return azure.PublicCloud, nil
	}

	return azure.EnvironmentFromName(cloudName)
}

// NewClientFromSecret returns a ABS client based on given k8s secret containing azure credentials.
func NewClientFromSecret(kubecli kubernetes.Interface, namespace, absSecret string) (w *ABSClient, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("new ABS client failed: %v", err)
		}
	}()

	se, err := kubecli.CoreV1().Secrets(namespace).Get(absSecret, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get k8s secret: %v", err)
	}

	storageAccount := se.Data[api.AzureSecretStorageAccount]
	storageKey := se.Data[api.AzureSecretStorageKey]
	cloudName := se.Data[api.AzureCloudKey]

	cloud, err := parseAzureEnvironment(string(cloudName))
	if err != nil {
		return nil, err
	}

	bc, err := storage.NewBasicClientOnSovereignCloud(
		string(storageAccount),
		string(storageKey),
		cloud)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure storage client: %v", err)
	}

	abs := bc.GetBlobService()
	return &ABSClient{ABS: &abs}, nil
}
