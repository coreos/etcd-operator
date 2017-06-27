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

package abs

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/Azure/azure-sdk-for-go/storage"
)

const (
	v1                        = "v1/"
	azureStorageAccountEnvVar = "AZURE_STORAGE_ACCOUNT"
	azureStorageKeyEnvVar     = "AZURE_STORAGE_KEY"
)

// ABS is a helper to wrap complex ABS logic
type ABS struct {
	container *storage.Container
	prefix    string
	client    *storage.BlobStorageClient
}

// New returns a new ABS object for a given container using credentials set in the environment
func New(container, prefix string) (*ABS, error) {
	accountName := os.Getenv(azureStorageAccountEnvVar)
	if accountName == "" {
		return nil, fmt.Errorf("missing required environment variable of %s", azureStorageAccountEnvVar)
	}
	accountKey := os.Getenv(azureStorageKeyEnvVar)
	if accountKey == "" {
		return nil, fmt.Errorf("missing required environment variable of %s", azureStorageKeyEnvVar)
	}
	basicClient, err := storage.NewBasicClient(accountName, accountKey)
	if err != nil {
		return nil, fmt.Errorf("create ABS client failed: %v", err)
	}

	return NewFromClient(container, prefix, &basicClient)
}

// NewFromClient returns a new ABS object for a given container using the supplied storageClient
func NewFromClient(container, prefix string, storageClient *storage.Client) (*ABS, error) {
	client := storageClient.GetBlobService()

	// Check if supplied container exists
	containerRef := client.GetContainerReference(container)
	containerExists, err := containerRef.Exists()
	if err != nil {
		return nil, err
	}
	if !containerExists {
		return nil, fmt.Errorf("container %v does not exist", container)
	}

	return &ABS{
		container: containerRef,
		prefix:    prefix,
		client:    &client,
	}, nil
}

// Put puts a chunk of data into a ABS container using the provided key for its reference
func (w *ABS) Put(key string, r io.Reader) error {
	blobName := path.Join(v1, w.prefix, key)
	blob := w.container.GetBlobReference(blobName)

	putBlobOpts := storage.PutBlobOptions{}
	err := blob.CreateBlockBlobFromReader(r, &putBlobOpts)
	if err != nil {
		return fmt.Errorf("create block blob from reader failed: %v", err)
	}

	return nil
}

// Get gets the blob object specified by key from a ABS container
func (w *ABS) Get(key string) (io.ReadCloser, error) {
	blobName := path.Join(v1, w.prefix, key)
	blob := w.container.GetBlobReference(blobName)

	opts := &storage.GetBlobOptions{}
	return blob.Get(opts)
}

// Delete deletes the blob object specified by key from a ABS container
func (w *ABS) Delete(key string) error {
	blobName := path.Join(v1, w.prefix, key)
	blob := w.container.GetBlobReference(blobName)

	opts := &storage.DeleteBlobOptions{}
	return blob.Delete(opts)
}

// List lists all blobs in a given ABS container
func (w *ABS) List() ([]string, error) {
	_, l, err := w.list(w.prefix)
	return l, err
}

func (w *ABS) list(prefix string) (int64, []string, error) {
	params := storage.ListBlobsParameters{Prefix: path.Join(v1, prefix) + "/"}
	resp, err := w.container.ListBlobs(params)
	if err != nil {
		return -1, nil, err
	}

	keys := []string{}
	var size int64
	for _, blob := range resp.Blobs {
		k := (blob.Name)[len(resp.Prefix):]
		keys = append(keys, k)
		size += blob.Properties.ContentLength
	}

	return size, keys, nil
}

// TotalSize returns the total size of all blobs in a ABS container
func (w *ABS) TotalSize() (int64, error) {
	size, _, err := w.list(w.prefix)
	return size, err
}

// CopyPrefix copies all blobs with given prefix
func (w *ABS) CopyPrefix(from string) error {
	_, blobs, err := w.list(from)
	if err != nil {
		return err
	}
	for _, blob := range blobs {
		blobResource := w.container.GetBlobReference(blob)

		opts := storage.CopyOptions{}
		if err = blobResource.Copy(path.Join(w.container.Name, v1, from, blob), &opts); err != nil {
			return err
		}
	}
	return nil
}
