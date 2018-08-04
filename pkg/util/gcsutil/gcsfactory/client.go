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
	"fmt"

	gcs "cloud.google.com/go/storage"
	"context"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudkms/v1"
	"google.golang.org/api/option"
	"k8s.io/client-go/kubernetes"
)

// GCSClient is a wrapper of GCS client that provides cleanup functionality.
type GCSClient struct {
	GCS *gcs.Client
}

// NewClientFromSecret returns a GCS client based on given k8s secret containing gcs credentials.
func NewClientFromSecret(_ kubernetes.Interface, _, _ string) (*GCSClient, error) {
	// implicit client
	httpClient, err := google.DefaultClient(context.Background(), cloudkms.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("failed to create google cloud platform client: %v", err)
	}
	c, err := gcs.NewClient(context.Background(), option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create gcs client with: %v", err)
	}
	log.Infof("Created an implicit GCS client")
	return &GCSClient{c}, nil
}
