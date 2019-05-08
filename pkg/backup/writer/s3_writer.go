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
	"fmt"
	"io"

	"github.com/coreos/etcd-operator/pkg/backup/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type s3Writer struct {
	s3 *s3.S3
}

// NewS3Writer creates a s3 writer.
func NewS3Writer(s3 *s3.S3) Writer {
	return &s3Writer{s3}
}

// Write writes the backup file to the given s3 path, "<s3-bucket-name>/<key>".
func (s3w *s3Writer) Write(ctx context.Context, path string, r io.Reader) (int64, error) {
	bk, key, err := util.ParseBucketAndKey(path)
	if err != nil {
		return 0, err
	}

	_, err = s3manager.NewUploaderWithClient(s3w.s3).UploadWithContext(ctx,
		&s3manager.UploadInput{
			Bucket: aws.String(bk),
			Key:    aws.String(key),
			Body:   r,
		})
	if err != nil {
		return 0, err
	}

	resp, err := s3w.s3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bk),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.ContentLength == nil {
		return 0, fmt.Errorf("failed to compute s3 object size")
	}
	return *resp.ContentLength, nil
}

// List return the file paths which match the given s3 path
func (s3w *s3Writer) List(ctx context.Context, basePath string) ([]string, error) {
	bk, key, err := util.ParseBucketAndKey(basePath)
	if err != nil {
		return nil, err
	}

	objects, err := s3w.s3.ListObjectsWithContext(ctx,
		&s3.ListObjectsInput{
			Bucket: aws.String(bk),
			Prefix: aws.String(key),
		})
	if err != nil {
		return nil, err
	}
	objectKeys := []string{}
	for _, object := range objects.Contents {
		objectKeys = append(objectKeys, bk+"/"+*object.Key)
	}
	return objectKeys, nil
}

func (s3w *s3Writer) Delete(ctx context.Context, path string) error {
	bk, key, err := util.ParseBucketAndKey(path)
	if err != nil {
		return err
	}

	_, err = s3w.s3.DeleteObjectWithContext(ctx,
		&s3.DeleteObjectInput{
			Bucket: aws.String(bk),
			Key:    aws.String(key),
		})
	return err
}
