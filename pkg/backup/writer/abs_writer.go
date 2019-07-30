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
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/coreos/etcd-operator/pkg/backup/util"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/pborman/uuid"
)

var _ Writer = &absWriter{}

type absWriter struct {
	abs *storage.BlobStorageClient
}

// NewABSWriter creates a abs writer.
func NewABSWriter(abs *storage.BlobStorageClient) Writer {
	return &absWriter{abs}
}

const (
	// AzureBlobBlockChunkLimitInBytes 100MiB is the limit
	AzureBlobBlockChunkLimitInBytes = 100 * 1024 * 1024
)

// Write writes the backup file to the given abs path, "<abs-container-name>/<key>".
func (absw *absWriter) Write(ctx context.Context, path string, r io.Reader) (int64, error) {
	// TODO: support context.
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
	err = blob.CreateBlockBlob(&storage.PutBlobOptions{})
	if err != nil {
		return 0, err
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	len := len(buf.Bytes())
	chunckCount := len/AzureBlobBlockChunkLimitInBytes + 1
	blocks := make([]storage.Block, 0, chunckCount)
	for i := 0; i < chunckCount; i++ {
		blockID := base64.StdEncoding.EncodeToString([]byte(uuid.New()))
		blocks = append(blocks, storage.Block{ID: blockID, Status: storage.BlockStatusLatest})
		start := i * AzureBlobBlockChunkLimitInBytes
		end := (i + 1) * AzureBlobBlockChunkLimitInBytes
		if len < end {
			end = len
		}

		chunk := buf.Bytes()[start:end]
		err = blob.PutBlock(blockID, chunk, &storage.PutBlockOptions{})
		if err != nil {
			return 0, err
		}
	}

	err = blob.PutBlockList(blocks, &storage.PutBlockListOptions{})
	if err != nil {
		return 0, err
	}

	_, err = blob.Get(&storage.GetBlobOptions{})
	if err != nil {
		return 0, err
	}

	return blob.Properties.ContentLength, nil
}

func (absw *absWriter) List(ctx context.Context, basePath string) ([]string, error) {
	// TODO: support context.
	container, key, err := util.ParseBucketAndKey(basePath)
	if err != nil {
		return nil, err
	}

	containerRef := absw.abs.GetContainerReference(container)
	containerExists, err := containerRef.Exists()
	if err != nil {
		return nil, err
	}
	if !containerExists {
		return nil, fmt.Errorf("container %v does not exist", container)
	}

	blobs, err := containerRef.ListBlobs(
		storage.ListBlobsParameters{Prefix: key})
	if err != nil {
		return nil, err
	}
	blobKeys := []string{}
	for _, blob := range blobs.Blobs {
		blobKeys = append(blobKeys, container+"/"+blob.Name)
	}
	return blobKeys, nil
}

func (absw *absWriter) Delete(ctx context.Context, path string) error {
	// TODO: support context.
	container, key, err := util.ParseBucketAndKey(path)
	if err != nil {
		return err
	}
	containerRef := absw.abs.GetContainerReference(container)
	containerExists, err := containerRef.Exists()
	if err != nil {
		return err
	}
	if !containerExists {
		return fmt.Errorf("container %v does not exist", container)
	}

	blob := containerRef.GetBlobReference(key)
	return blob.Delete(&storage.DeleteBlobOptions{})
}
