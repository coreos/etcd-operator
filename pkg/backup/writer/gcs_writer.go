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
	"context"
	"io"

	"cloud.google.com/go/storage"

	"github.com/coreos/etcd-operator/pkg/backup/util"
	log "github.com/sirupsen/logrus"
)

type gcsWriter struct {
	gcs *storage.Client
}

// NewS3Writer creates a s3 writer.
func NewGCSWriter(gcs *storage.Client) Writer {
	return &gcsWriter{
		gcs: gcs,
	}
}

// Write writes the backup file to the given gcs path, "<gcs-bucket-name>/<key>".
func (g *gcsWriter) Write(ctx context.Context, path string, r io.Reader) (int64, error) {
	bucketName, keyPath, err := util.ParseBucketAndKey(path)
	if err != nil {
		return 0, err
	}

	bucketWriter := g.gcs.Bucket(bucketName).Object(keyPath).NewWriter(ctx)
	log.Infof("Writing etcd snapshot in bucket %s as object name %s", bucketName, keyPath)
	n, err := io.Copy(bucketWriter, r)
	if err != nil {
		log.Errorf("Failed to write the etcd backup to the bucket %s at the key %s: %v", bucketName, keyPath, err)
		return 0, err
	}
	// GCS actually sync data to the bucket on close
	err = bucketWriter.Close()
	if err != nil {
		log.Errorf("Unexpected error while writing %d bytes to the bucket %s as object name %s", n, bucketName, keyPath, err)
		return 0, err
	}
	log.Infof("Wrote %d bytes to the bucket %s at the key %s", n, bucketName, keyPath)
	return int64(n), nil
}
