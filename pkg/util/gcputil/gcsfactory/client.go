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

package gcsfactory

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GCSClient is a wrapper of GCS client that provides cleanup functionality.
type GCSClient struct {
	GCS *storage.Client
}

// NewClientFromSecret returns a GCS client based on given k8s secret containing azure credentials.
func NewClientFromSecret(ctx context.Context, kubecli kubernetes.Interface, namespace, gcsSecret string) (w *GCSClient, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("new GCS client failed: %v", err)
		}
	}()

	var authOptions []option.ClientOption
	if se, err := kubecli.CoreV1().Secrets(namespace).Get(gcsSecret, metav1.GetOptions{}); err == nil {
		if accessToken, ok := se.Data[api.GCPAccessToken]; ok {
			authOptions = append(authOptions, option.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: string(accessToken)})))
		} else if credentialsJson, ok := se.Data[api.GCPCredentialsJson]; ok {
			authOptions = append(authOptions, option.WithCredentialsJSON(credentialsJson))
		}
	}

	gcs, err := storage.NewClient(ctx, authOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return &GCSClient{GCS: gcs}, nil
}
