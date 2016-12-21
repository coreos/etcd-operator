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
	"crypto/rand"
	"encoding/base64"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coreos/etcd-operator/pkg/backup/s3"
)

func TestS3BackendPurge(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TEST") != "true" {
		t.Skip("skipping integration test due to RUN_INTEGRATION_TEST not set")
	}
	prefix := randString(8) + "/"
	s3cli, err := s3.New("test-bucket", prefix, session.Options{
		Config: aws.Config{
			Credentials:      credentials.NewStaticCredentials(os.Getenv("MINIO_ACCESS_KEY_ID"), os.Getenv("MINIO_SECRET_ACCESS_KEY"), ""),
			Endpoint:         aws.String("http://localhost:9000"),
			Region:           aws.String("us-east-1"),
			DisableSSL:       aws.Bool(true),
			S3ForcePathStyle: aws.Bool(true),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "etcd-operator-test")
	if err != nil {
		t.Fatal(err)
	}
	s := &s3Backend{
		S3:  s3cli,
		dir: dir,
	}
	if err := s.save("3.1.0", 1, bytes.NewBuffer([]byte("ignore"))); err != nil {
		t.Fatal(err)
	}
	if err := s.save("3.1.0", 2, bytes.NewBuffer([]byte("ignore"))); err != nil {
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

func randString(l int) string {
	b := make([]byte, l)
	rand.Read(b)
	en := base64.StdEncoding
	d := make([]byte, en.EncodedLen(len(b)))
	en.Encode(d, b)
	return string(d)
}
