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

package e2e

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

// TestBackupAndRestore runs the backup test first, and only runs the restore test after if the backup test succeeds and sets the S3 path
func TestBackupAndRestore(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	if err := verifyAWSEnvVars(); err != nil {
		t.Fatal(err)
	}
	s3Path := testEtcdBackupOperatorForS3Backup(t)
	if len(s3Path) == 0 {
		t.Fatal("skipping restore test: S3 path not set despite testEtcdBackupOperatorForS3Backup success")
	}
	testEtcdRestoreOperatorForS3Source(t, s3Path)
}

func verifyAWSEnvVars() error {
	if len(os.Getenv("TEST_S3_BUCKET")) == 0 {
		return fmt.Errorf("TEST_S3_BUCKET not set")
	}
	if len(os.Getenv("TEST_AWS_SECRET")) == 0 {
		return fmt.Errorf("TEST_AWS_SECRET not set")
	}
	return nil
}

// testEtcdBackupOperatorForS3Backup tests if etcd backup operator can save etcd backup to S3.
// It returns the full S3 path where the backup is saved.
func testEtcdBackupOperatorForS3Backup(t *testing.T) string {
	f := framework.Global
	suffix := fmt.Sprintf("-%d", rand.Uint64())
	clusterName := "test-etcd-backup-" + suffix
	memberPeerTLSSecret := "etcd-peer-tls" + suffix
	memberClientTLSSecret := "etcd-server-tls" + suffix
	operatorClientTLSSecret := "etcd-client-tls" + suffix

	err := e2eutil.PrepareTLS(clusterName, f.Namespace, memberPeerTLSSecret, memberClientTLSSecret, operatorClientTLSSecret)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := e2eutil.DeleteSecrets(f.KubeClient, f.Namespace, memberPeerTLSSecret, memberClientTLSSecret, operatorClientTLSSecret)
		if err != nil {
			t.Fatal(err)
		}
	}()

	c := e2eutil.NewCluster("", 3)
	c.Name = clusterName
	e2eutil.ClusterCRWithTLS(c, memberPeerTLSSecret, memberClientTLSSecret, operatorClientTLSSecret)
	testEtcd, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, c)

	defer func() {
		if err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := e2eutil.WaitUntilSizeReached(t, f.CRClient, 3, 6, testEtcd); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}
	backCR := e2eutil.NewS3Backup(testEtcd.Name, os.Getenv("TEST_S3_BUCKET"), os.Getenv("TEST_AWS_SECRET"), operatorClientTLSSecret)
	eb, err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Create(backCR)
	if err != nil {
		t.Fatalf("failed to create etcd backup cr: %v", err)
	}
	defer func() {
		if err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Delete(eb.Name, nil); err != nil {
			t.Fatalf("failed to delete etcd backup cr: %v", err)
		}
	}()

	// local testing shows that it takes around 1 - 2 seconds from creating backup cr to verifying the backup from s3.
	// 4 seconds timeout via retry is enough; duration longer than that may indicate internal issues and
	// is worthy of investigation.
	s3Path := ""
	s3cli, err := s3factory.NewClientFromSecret(f.KubeClient, f.Namespace, os.Getenv("TEST_AWS_SECRET"))
	if err != nil {
		t.Fatalf("failed create s3 client: %v", err)
	}
	defer s3cli.Close()
	err = retryutil.Retry(time.Second, 4, func() (bool, error) {
		reb, err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Get(eb.Name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to retrieve backup CR: %v", err)
		}
		if reb.Status.Succeeded {
			// bucketAndKey[0] holds s3 bucket name.
			// bucketAndKey[1] holds the s3 object path without the prefixed bucket name.
			bucketAndKey := strings.SplitN(reb.Status.S3Path, "/", 2)
			_, err := s3cli.S3.GetObject(&s3.GetObjectInput{
				Bucket: aws.String(bucketAndKey[0]),
				Key:    aws.String(bucketAndKey[1]),
			})
			if err != nil {
				return false, fmt.Errorf("failed to get backup %v from s3 : %v", reb.Status.S3Path, err)
			}
			s3Path = reb.Status.S3Path
			return true, nil
		} else if len(reb.Status.Reason) != 0 {
			return false, fmt.Errorf("backup failed with reason: %v ", reb.Status.Reason)
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to verify backup: %v", err)
	}
	return s3Path
}

// testEtcdRestoreOperatorForS3Source tests if the restore-operator can restore an etcd cluster from an S3 restore source
func testEtcdRestoreOperatorForS3Source(t *testing.T, s3Path string) {
	f := framework.Global

	restoreSource := api.RestoreSource{S3: e2eutil.NewS3RestoreSource(s3Path, os.Getenv("TEST_AWS_SECRET"))}
	er := e2eutil.NewEtcdRestore("test-etcd-restore-", "3.2.11", 3, restoreSource)
	er, err := f.CRClient.EtcdV1beta2().EtcdRestores(f.Namespace).Create(er)
	if err != nil {
		t.Fatalf("failed to create etcd restore cr: %v", err)
	}
	defer func() {
		if err := f.CRClient.EtcdV1beta2().EtcdRestores(f.Namespace).Delete(er.Name, nil); err != nil {
			t.Fatalf("failed to delete etcd restore cr: %v", err)
		}
	}()

	// Verify the EtcdRestore CR status "succeeded=true". In practice the time taken to update is 1 second.
	err = retryutil.Retry(time.Second, 5, func() (bool, error) {
		er, err := f.CRClient.EtcdV1beta2().EtcdRestores(f.Namespace).Get(er.Name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to retrieve restore CR: %v", err)
		}
		if er.Status.Succeeded {
			return true, nil
		} else if len(er.Status.Reason) != 0 {
			return false, fmt.Errorf("restore failed with reason: %v ", er.Status.Reason)
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to verify restore succeeded: %v", err)
	}

	// Verify that the restored etcd cluster scales to 3 ready members
	restoredCluster := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      er.Name,
			Namespace: f.Namespace,
		},
		Spec: api.ClusterSpec{
			Size: 3,
		},
	}
	if _, err := e2eutil.WaitUntilSizeReached(t, f.CRClient, 3, 6, restoredCluster); err != nil {
		t.Fatalf("failed to see restored etcd cluster(%v) reach 3 members: %v", restoredCluster.Name, err)
	}
	if err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, restoredCluster); err != nil {
		t.Fatalf("failed to delete restored cluster(%v): %v", restoredCluster.Name, err)
	}
}
