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
	"io"

	"github.com/coreos/etcd-operator/pkg/backup/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type s3Writer struct {
	u *s3manager.Uploader
}

// NewS3Writer creates a s3 writer.
func NewS3Writer(s3 *s3.S3) Writer {
	return &s3Writer{s3manager.NewUploaderWithClient(s3)}
}

// Write writes the backup file to the given s3 path, "<s3-bucket-name>/<key>".
func (s3w *s3Writer) Write(path string, r io.Reader) error {
	bk, key, err := util.ParseBucketAndKey(path)
	if err != nil {
		return err
	}

	_, err = s3w.u.Upload(
		&s3manager.UploadInput{
			Bucket: aws.String(bk),
			Key:    aws.String(key),
			Body:   r,
		})
	return err
}
