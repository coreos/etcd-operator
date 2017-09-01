// Copyright 2016 The etcd-operator Authors
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

package backup

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coreos/etcd-operator/pkg/backup/s3"
)

var r *rand.Rand // Rand for this package.

func init() {
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func randString(strlen int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, strlen)
	for i := range result {
		result[i] = chars[r.Intn(len(chars))]
	}
	return string(result)
}

func TestS3Backend(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TEST") != "true" {
		t.Skip("skipping integration test due to RUN_INTEGRATION_TEST not set")
	}
	rs := randString(10)
	sessOpt := session.Options{
		Config: aws.Config{
			Credentials:      credentials.NewStaticCredentials(os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY"), ""),
			Endpoint:         aws.String("http://localhost:9000"),
			Region:           aws.String("us-east-1"),
			DisableSSL:       aws.Bool(true),
			S3ForcePathStyle: aws.Bool(true),
		},
	}
	buc := "test-bucket" // This is pre-set bucket
	s3cli, err := s3.NewFromSessionOpt(buc, rs, sessOpt)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("test S3 backend purge", func(t *testing.T) {
		testS3BackendPurge(t, s3cli)
	})
	t.Run("test S3 backend delete", func(t *testing.T) {
		testS3BackendDelete(t, s3cli)
	})
}

func testS3BackendPurge(t *testing.T, s3cli *s3.S3) {
	dir, err := ioutil.TempDir("", "etcd-operator-test")
	if err != nil {
		t.Fatal(err)
	}
	s := &s3Backend{
		S3:  s3cli,
		dir: dir,
	}
	if _, err := s.save("3.1.0", 1, bytes.NewBuffer([]byte("ignore"))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.save("3.1.0", 2, bytes.NewBuffer([]byte("ignore"))); err != nil {
		t.Fatal(err)
	}
	if err := s.purge(1); err != nil {
		t.Fatal(err)
	}
	names, err := s3cli.List()
	if err != nil {
		t.Fatal(err)
	}
	leftFiles := []string{makeBackupName("3.1.0", 2)}
	if !reflect.DeepEqual(leftFiles, names) {
		t.Errorf("left files after purge, want=%v, get=%v", leftFiles, names)
	}
	if err := s3cli.Delete(makeBackupName("3.1.0", 2)); err != nil {
		t.Fatal(err)
	}
}

func testS3BackendDelete(t *testing.T, s3cli *s3.S3) {
	dir, err := ioutil.TempDir("", "etcd-operator-test")
	if err != nil {
		t.Fatal(err)
	}
	s := &s3Backend{
		S3:  s3cli,
		dir: dir,
	}
	if _, err := s.save("3.1.0", 1, bytes.NewBuffer([]byte("ignore"))); err != nil {
		t.Fatal(err)
	}
	names, err := s3cli.List()
	if err != nil {
		t.Fatal(err)
	}
	leftFiles := []string{makeBackupName("3.1.0", 1)}
	if !reflect.DeepEqual(leftFiles, names) {
		t.Errorf("left files after purge, want=%v, get=%v", leftFiles, names)
	}
	if err := s3cli.Delete(makeBackupName("3.1.0", 1)); err != nil {
		t.Fatal(err)
	}
	names, err = s3cli.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("backup files should have been deleted, but get=%v", names)
	}
}

// TestS3BackendWithPrefix ensures prefixed s3 clients do save files under the prefixed path.
func TestS3BackendWithPrefix(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TEST") != "true" {
		t.Skip("skipping integration test due to RUN_INTEGRATION_TEST not set")
	}
	sessOpt := session.Options{
		Config: aws.Config{
			Credentials:      credentials.NewStaticCredentials(os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY"), ""),
			Endpoint:         aws.String("http://localhost:9000"),
			Region:           aws.String("us-east-1"),
			DisableSSL:       aws.Bool(true),
			S3ForcePathStyle: aws.Bool(true),
		},
	}
	buc := "test-bucket" // This is pre-set bucket
	prefix := randString(10)
	s3Cli, err := s3.NewFromSessionOpt(buc, prefix, sessOpt)
	if err != nil {
		t.Fatal(err)
	}
	prefix2 := randString(10)
	s3Cli2, err := s3.NewFromSessionOpt(buc, prefix2, sessOpt)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "etcd-operator-test")
	defer os.RemoveAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := &s3Backend{
		S3:  s3Cli,
		dir: dir,
	}
	s2 := &s3Backend{
		S3:  s3Cli2,
		dir: dir,
	}

	if _, err := s.save("file1", 1, bytes.NewBuffer([]byte("ignore"))); err != nil {
		t.Fatal(err)
	}

	if _, err := s2.save("file2", 1, bytes.NewBuffer([]byte("ignore"))); err != nil {
		t.Fatal(err)
	}

	// verify s1 only have one file under prefix "prefix"
	total, err := s.total()
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expect 1 file, but got %v", total)
	}

	// verify s2 only have one file under prefix "prefix2"
	total, err = s2.total()
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expect 1 file, but got %v", total)
	}

	// clean up
	if err = s.purge(1); err != nil {
		t.Fatal(err)
	}

	if err = s2.purge(1); err != nil {
		t.Fatal(err)
	}
}
