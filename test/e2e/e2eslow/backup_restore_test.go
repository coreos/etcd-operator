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

package e2eslow

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup/writer"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

// TestBackupAndRestore runs the backup test first, and only runs the restore test after if the backup test succeeds and sets the S3 path
func TestBackupAndRestore(t *testing.T) {
	if err := verifyAWSEnvVars(); err != nil {
		t.Fatal(err)
	}

	// Create cluster with TLS
	f := framework.Global
	suffix := fmt.Sprintf("%d", rand.Uint64())
	clusterName := "test-etcd-backup-restore-" + suffix
	memberPeerTLSSecret := "etcd-peer-tls-" + suffix
	memberClientTLSSecret := "etcd-server-tls-" + suffix
	operatorClientTLSSecret := "etcd-client-tls-" + suffix
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

	s3Path := path.Join(os.Getenv("TEST_S3_BUCKET"), "jenkins", suffix, time.Now().Format(time.RFC3339), "etcd.backup")

	// Backup then restore tests
	testEtcdBackupOperatorForS3Backup(t, clusterName, operatorClientTLSSecret, s3Path)
	testEtcdRestoreOperatorForS3Source(t, clusterName, s3Path)
	// Periodic backup test
	testEtcdBackupOperatorForPeriodicS3Backup(t, clusterName, operatorClientTLSSecret, s3Path)
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

func getEndpoints(kubeClient kubernetes.Interface, secureClient bool, namespace, clusterName string) ([]string, error) {
	podList, err := kubeClient.Core().Pods(namespace).List(k8sutil.ClusterListOpt(clusterName))
	if err != nil {
		return nil, err
	}

	var pods []*v1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == v1.PodRunning {
			pods = append(pods, pod)
		}
	}

	if len(pods) == 0 {
		return nil, errors.New("no running etcd pods found")
	}

	endpoints := make([]string, len(pods))
	for i, pod := range pods {
		m := &etcdutil.Member{
			Name:         pod.Name,
			Namespace:    pod.Namespace,
			SecureClient: secureClient,
		}
		endpoints[i] = m.ClientURL()
	}
	return endpoints, nil
}

// testEtcdBackupOperatorForS3Backup tests if etcd backup operator can save etcd backup to S3.
// It returns the full S3 path where the backup is saved.
func testEtcdBackupOperatorForS3Backup(t *testing.T, clusterName, operatorClientTLSSecret, s3Path string) {
	f := framework.Global

	endpoints, err := getEndpoints(f.KubeClient, true, f.Namespace, clusterName)
	if err != nil {
		t.Fatalf("failed to get endpoints: %v", err)
	}
	backupCR := e2eutil.NewS3Backup(endpoints, clusterName, s3Path, os.Getenv("TEST_AWS_SECRET"), operatorClientTLSSecret)
	eb, err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Create(backupCR)
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
	err = retryutil.Retry(time.Second, 4, func() (bool, error) {
		reb, err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Get(eb.Name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to retrieve backup CR: %v", err)
		}
		if reb.Status.Succeeded {
			if reb.Status.EtcdVersion == api.DefaultEtcdVersion && reb.Status.EtcdRevision == 1 {
				return true, nil
			}
			return false, fmt.Errorf("expect EtcdVersion==%v and EtcdRevision==1, but got EtcdVersion==%v and EtcdRevision==%v", api.DefaultEtcdVersion, reb.Status.EtcdVersion, reb.Status.EtcdRevision)
		}
		if len(reb.Status.Reason) != 0 {
			return false, fmt.Errorf("backup failed with reason: %v ", reb.Status.Reason)
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to verify backup: %v", err)
	}
	t.Logf("backup for cluster (%s) has been saved", clusterName)
}

// testEtcdBackupOperatorForPeriodicS3Backup test if etcd backup operator can periodically backup and upload to s3.
// This e2e test would check basic function of periodic backup and MaxBackup functionality
func testEtcdBackupOperatorForPeriodicS3Backup(t *testing.T, clusterName, operatorClientTLSSecret, s3Path string) {
	f := framework.Global

	endpoints, err := getEndpoints(f.KubeClient, true, f.Namespace, clusterName)
	if err != nil {
		t.Fatalf("failed to get endpoints: %v", err)
	}
	backupCR := e2eutil.NewS3Backup(endpoints, clusterName, s3Path, os.Getenv("TEST_AWS_SECRET"), operatorClientTLSSecret)
	// enable periodic backup
	backupCR.Spec.BackupPolicy = &api.BackupPolicy{BackupIntervalInSecond: 5, MaxBackups: 2}
	backupS3Source := backupCR.Spec.BackupSource.S3

	// initialize s3 client
	s3cli, err := s3factory.NewClientFromSecret(
		f.KubeClient, f.Namespace, backupS3Source.Endpoint, backupS3Source.AWSSecret, backupS3Source.ForcePathStyle)
	if err != nil {
		t.Fatalf("failed to initialize s3client: %v", err)
	}
	wr := writer.NewS3Writer(s3cli.S3)

	// check if there is existing backup file
	allBackups, err := wr.List(context.Background(), backupS3Source.Path)
	if err != nil {
		t.Fatalf("failed to list backup files: %v", err)
	}
	if len(allBackups) > 0 {
		t.Logf("existing backup file is detected: %s", strings.Join(allBackups, ","))
		// try to delete all existing backup files
		if err := e2eutil.DeleteBackupFiles(wr, allBackups); err != nil {
			t.Fatalf("failed to delete existing backup: %v", err)
		}
		// make sure no exisiting backups
		// will wait for 10 sec until deleting operation completed
		if err := e2eutil.WaitUntilNoBackupFiles(wr, backupS3Source.Path, 10); err != nil {
			t.Fatalf("failed to make sure no old backup: %v", err)
		}
	}

	// create etcdbackup resource
	eb, err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Create(backupCR)
	if err != nil {
		t.Fatalf("failed to create etcd back cr: %v", err)
	}
	defer func() {
		if err := f.CRClient.EtcdV1beta2().EtcdBackups(f.Namespace).Delete(eb.Name, nil); err != nil {
			t.Fatalf("failed to delete etcd backup cr: %v", err)
		}
		// cleanup backup files
		allBackups, err = wr.List(context.Background(), backupS3Source.Path)
		if err != nil {
			t.Fatalf("failed to list backup files: %v", err)
		}
		if err := e2eutil.DeleteBackupFiles(wr, allBackups); err != nil {
			t.Fatalf("failed to cleanup backup files: %v", err)
		}
	}()

	var firstBackup string
	var periodicBackup, maxBackup bool
	// Check if periodic backup is correctly performed
	// Check if maxBackup is correctly performed
	err = retryutil.Retry(time.Second, 20, func() (bool, error) {
		allBackups, err = wr.List(context.Background(), backupS3Source.Path)
		sort.Strings(allBackups)
		if err != nil {
			return false, fmt.Errorf("failed to list backup files: %v", err)
		}
		if len(allBackups) > 0 {
			if firstBackup == "" {
				firstBackup = allBackups[0]
			}
			// Check if firt seen backup file is deleted or not
			if firstBackup != allBackups[0] {
				maxBackup = true
			}
			if len(allBackups) > 1 {
				periodicBackup = true
			}
		}
		if periodicBackup && maxBackup {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to verify periodic bakcup: %v", err)
	}
}

// testEtcdRestoreOperatorForS3Source tests if the restore-operator can restore an etcd cluster from an S3 restore source
func testEtcdRestoreOperatorForS3Source(t *testing.T, clusterName, s3Path string) {
	f := framework.Global

	restoreSource := api.RestoreSource{S3: e2eutil.NewS3RestoreSource(s3Path, os.Getenv("TEST_AWS_SECRET"))}
	er := e2eutil.NewEtcdRestore(clusterName, 3, restoreSource, api.BackupStorageTypeS3)
	er, err := f.CRClient.EtcdV1beta2().EtcdRestores(f.Namespace).Create(er)
	if err != nil {
		t.Fatalf("failed to create etcd restore cr: %v", err)
	}
	defer func() {
		if err := f.CRClient.EtcdV1beta2().EtcdRestores(f.Namespace).Delete(er.Name, nil); err != nil {
			t.Fatalf("failed to delete etcd restore cr: %v", err)
		}
	}()

	err = retryutil.Retry(10*time.Second, 1, func() (bool, error) {
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
			Name:      clusterName,
			Namespace: f.Namespace,
		},
		Spec: api.ClusterSpec{
			Size: 3,
		},
	}
	if _, err := e2eutil.WaitUntilSizeReached(t, f.CRClient, 3, 6, restoredCluster); err != nil {
		t.Fatalf("failed to see restored etcd cluster(%v) reach 3 members: %v", restoredCluster.Name, err)
	}
}
