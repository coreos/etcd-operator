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
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// S3 represents AWS S3 service.
type S3 struct {
	bucket string
	client *s3.S3
}

// AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set to access AWS services
// AWS_S3_BUCKET must be set to specify the s3 bucket
// AWS_REGION can be set to specify the aws region
func New() (*S3, error) {
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("AWS_S3_BUCKET bucket must be set")
	}

	client := s3.New(session.New())

	_, err := client.HeadBucket(&s3.HeadBucketInput{Bucket: &bucket})
	if err != nil {
		return nil, fmt.Errorf("unable to access bucket %s: %v", bucket, err)
	}

	s := &S3{
		client: client,
		bucket: bucket,
	}
	return s, nil
}

func (s *S3) Put(key string, rs io.ReadSeeker) error {
	_, err := s.client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   rs,
	})

	if err != nil {
		return err
	}

	return nil
}

func (s *S3) Get(key string) (io.ReadCloser, error) {
	resp, err := s.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (s *S3) Delete(key string) error {
	_, err := s.client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return err
	}

	return nil
}

func (s *S3) List(prefix string) ([]string, error) {
	resp, err := s.client.ListObjects(&s3.ListObjectsInput{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, err
	}

	keys := []string{}
	for _, key := range resp.Contents {
		keys = append(keys, *key.Key)
	}

	return keys, nil
}
