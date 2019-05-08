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
	"fmt"
	"io"

	"github.com/coreos/etcd-operator/pkg/backup/util"

	"cloud.google.com/go/storage"
)

// ensure gcsReader satisfies reader interface.
var _ Reader = &gcsReader{}

// gcsReader provides Reader implementation for reading a file from GCS
type gcsReader struct {
	ctx context.Context
	gcs *storage.Client
}

// NewGCSReader return a Reader implementation to read a file from GCS in the form of gcsReader
func NewGCSReader(ctx context.Context, gcs *storage.Client) Reader {
	return &gcsReader{ctx, gcs}
}

// Open opens the file on path where path must be in the format "<gcs-bucket-name>/<key>"
func (gcsr *gcsReader) Open(path string) (io.ReadCloser, error) {
	bucket, key, err := util.ParseBucketAndKey(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse gcs bucket and key: %v", err)
	}

	return gcsr.gcs.Bucket(bucket).Object(key).NewReader(gcsr.ctx)
}
