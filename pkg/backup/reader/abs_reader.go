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

package reader

import (
	"fmt"
	"io"

	"github.com/coreos/etcd-operator/pkg/backup/util"

	"github.com/Azure/azure-sdk-for-go/storage"
)

// ensure absReader satisfies reader interface.
var _ Reader = &absReader{}

// absReader provides Reader implementation for reading a file from ABS
type absReader struct {
	abs *storage.BlobStorageClient
}

// NewABSReader return a Reader implementation to read a file from ABS in the form of absReader
func NewABSReader(abs *storage.BlobStorageClient) Reader {
	return &absReader{abs}
}

// Open opens the file on path where path must be in the format "<abs-container-name>/<key>"
func (absr *absReader) Open(path string) (io.ReadCloser, error) {
	container, key, err := util.ParseBucketAndKey(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse abs container and key: %v", err)
	}

	containerRef := absr.abs.GetContainerReference(container)
	containerExists, err := containerRef.Exists()
	if err != nil {
		return nil, err
	}

	if !containerExists {
		return nil, fmt.Errorf("container %v does not exist", container)
	}

	blob := containerRef.GetBlobReference(key)
	return blob.Get(&storage.GetBlobOptions{})
}
