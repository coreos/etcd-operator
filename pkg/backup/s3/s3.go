// Copyright 2016 The kube-etcd-etcd-operator Authors
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

package s3

import (
	"fmt"
	"io"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const (
	v1 = "v1/"
)

// S3 is a helper layer to wrap complex S3 logic.
type S3 struct {
	bucket string
	prefix string
	client *s3.S3
}

// New returns a S3 translator from default shared config.
// Please refer to http://docs.aws.amazon.com/sdk-for-go/api/aws/session/
// for setting up credentials and configuration.
func New(bucket, pathPrefix, prefix string) (*S3, error) {
	return NewFromSessionOpt(bucket, pathPrefix, prefix, session.Options{
		// Setting this is equal to the AWS_SDK_LOAD_CONFIG environment variable was set.
		// We want to save the work to set AWS_SDK_LOAD_CONFIG=1 outside.
		SharedConfigState: session.SharedConfigEnable,
	})
}

func NewFromSessionOpt(bucket, pathPrefix, prefix string, so session.Options) (*S3, error) {
	sess, err := session.NewSessionWithOptions(so)
	if err != nil {
		return nil, fmt.Errorf("new AWS session failed: %v", err)
	}
	cli := s3.New(sess)

	return NewFromClient(bucket, pathPrefix, prefix, cli), nil
}

func NewFromClient(bucket, pathPrefix, prefix string, cli *s3.S3) *S3 {
	return &S3{
		bucket: bucket,
		prefix: path.Join(pathPrefix, v1, prefix),
		client: cli,
	}
}

func (s *S3) Put(key string, rs io.ReadSeeker) error {
	_, err := s.client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path.Join(s.prefix, key)),
		Body:   rs,
	})

	return err
}

func (s *S3) Get(key string) (io.ReadCloser, error) {
	resp, err := s.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path.Join(s.prefix, key)),
	})
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (s *S3) Delete(key string) error {
	_, err := s.client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path.Join(s.prefix, key)),
	})

	return err
}

func (s *S3) List() ([]string, error) {
	_, l, err := s.list(s.prefix)
	return l, err
}

func (s *S3) list(prefix string) (int64, []string, error) {
	resp, err := s.client.ListObjects(&s3.ListObjectsInput{
		Bucket: aws.String(s.bucket),
		// s3 doesn't have dir. It only recognizes prefix.
		// Thus "a/b" has prefix "a/"
		Prefix: aws.String(prefix + "/"),
	})
	if err != nil {
		return -1, nil, err
	}

	keys := []string{}
	var size int64
	for _, key := range resp.Contents {
		k := (*key.Key)[len(*resp.Prefix):]
		keys = append(keys, k)
		size += *key.Size
	}

	return size, keys, nil
}

func (s *S3) TotalSize() (int64, error) {
	size, _, err := s.list(s.prefix)
	return size, err
}

func (s *S3) CopyPrefix(from string) error {
	_, keys, err := s.list(from)
	if err != nil {
		return err
	}
	for _, key := range keys {
		req := &s3.CopyObjectInput{
			Bucket:     aws.String(s.bucket),
			Key:        aws.String(path.Join(s.prefix, key)),
			CopySource: aws.String(path.Join(s.bucket, v1, from, key)),
		}
		_, err := s.client.CopyObject(req)
		if err != nil {
			return err
		}
	}
	return nil
}
