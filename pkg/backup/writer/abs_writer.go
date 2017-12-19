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

package writer

import (
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/coreos/etcd-operator/pkg/backup/util"
)

var _ Writer = &absWriter{}

type absWriter struct {
	abs *storage.BlobStorageClient
}

// NewABSWriter creates a abs writer.
func NewABSWriter(abs *storage.BlobStorageClient) Writer {
	return &absWriter{abs}
}

// Write writes the backup file to the given abs path, "<abs-container-name>/<key>".
func (absw *absWriter) Write(path string, r io.Reader) (int64, error) {
	container, key, err := util.ParseBucketAndKey(path)
	if err != nil {
		return 0, err
	}

	containerRef := absw.abs.GetContainerReference(container)
	containerExists, err := containerRef.Exists()
	if err != nil {
		return 0, err
	}

	if !containerExists {
		return 0, fmt.Errorf("container %v does not exist", container)
	}

	blob := containerRef.GetBlobReference(key)
	putBlobOpts := storage.PutBlobOptions{}
	err = blob.CreateBlockBlobFromReader(r, &putBlobOpts)
	if err != nil {
		return 0, fmt.Errorf("create block blob from reader failed: %v", err)
	}

	getBlobOpts := &storage.GetBlobOptions{}
	_, err = blob.Get(getBlobOpts)
	if err != nil {
		return 0, err
	}

	return blob.Properties.ContentLength, nil
}
