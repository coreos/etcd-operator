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
	"context"
	"io"

	"cloud.google.com/go/storage"
	"github.com/coreos/etcd-operator/pkg/backup/util"
)

// ensure gcsReader satisfies reader interface.
var _ Reader = &gcsReader{}

type gcsReader struct {
	gcs *storage.Client
}

// NewS3Writer creates a s3 writer.
func NewGCSReader(gcs *storage.Client) Reader {
	return &gcsReader{
		gcs: gcs,
	}
}

// Write writes the backup file to the given gcs path, "<gcs-bucket-name>/<key>".
func (g *gcsReader) Open(path string) (io.ReadCloser, error) {
	bucketName, keyPath, err := util.ParseBucketAndKey(path)
	if err != nil {
		return nil, err
	}
	return g.gcs.Bucket(bucketName).Object(keyPath).NewReader(context.Background())
}
