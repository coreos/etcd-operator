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

package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/test/e2e/framework"

	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterRestore(t *testing.T) {
	if os.Getenv(framework.EnvCloudProvider) == "aws" {
		t.Skip("skipping test due to relying on PodIP reachability. TODO: Remove this skip later")
	}

	t.Run("restore cluster from backup", func(t *testing.T) {
		t.Run("restore from the same name cluster", testClusterRestoreSameName)
		t.Run("restore from a different name cluster", testClusterRestoreDifferentName)
	})
}

func TestClusterRestoreS3(t *testing.T) {
	if os.Getenv(framework.EnvCloudProvider) == "aws" {
		t.Skip("skipping test due to relying on PodIP reachability. TODO: Remove this skip later")
	}

	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}

	t.Run("restore cluster from backup on S3", func(t *testing.T) {
		t.Run("restore from the same name cluster", testClusterRestoreS3SameName)
		t.Run("restore from a different name cluster", testClusterRestoreS3DifferentName)
	})
}

func testClusterRestoreSameName(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testClusterRestore(t, false)
}

func testClusterRestoreDifferentName(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	testClusterRestore(t, true)
}

func testClusterRestore(t *testing.T, needDataClone bool) {
	testClusterRestoreWithBackupPolicy(t, needDataClone, newBackupPolicyPV())
}

func testClusterRestoreWithBackupPolicy(t *testing.T, needDataClone bool, backupPolicy *spec.BackupPolicy) {
	f := framework.Global

	origEtcd := newClusterSpec("test-etcd-", 3)
	testEtcd, err := createCluster(t, f, etcdClusterWithBackup(origEtcd, backupPolicy))
	if err != nil {
		t.Fatal(err)
	}

	names, err := waitUntilSizeReached(t, f, testEtcd.Metadata.Name, 3, 60*time.Second)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	pod, err := f.KubeClient.CoreV1().Pods(f.Namespace).Get(names[0], metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	err = putDataToEtcd(fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
	if err != nil {
		t.Fatal(err)
	}

	if err := waitBackupPodUp(f, testEtcd.Metadata.Name, 60*time.Second); err != nil {
		t.Fatalf("failed to create backup pod: %v", err)
	}
	if err := makeBackup(f, testEtcd.Metadata.Name); err != nil {
		t.Fatalf("fail to make a backup: %v", err)
	}
	if err := deleteEtcdCluster(t, f, testEtcd); err != nil {
		t.Fatal(err)
	}
	// waits a bit to make sure resources are finally deleted on APIServer.
	time.Sleep(5 * time.Second)

	if !needDataClone {
		// Restore the etcd cluster of the same name:
		// - use the name already generated. We don't need to regenerate again.
		// - set BackupClusterName to the same name in RestorePolicy.
		// Then operator will use the existing backup in the same storage and
		// restore cluster with the same data.
		origEtcd.Metadata.GenerateName = ""
		origEtcd.Metadata.Name = testEtcd.Metadata.Name
	}
	waitRestoreTimeout := calculateRestoreWaitTime(needDataClone)

	origEtcd = etcdClusterWithRestore(origEtcd, &spec.RestorePolicy{
		BackupClusterName: testEtcd.Metadata.Name,
		StorageType:       backupPolicy.StorageType,
	})

	backupPolicy.CleanupBackupsOnClusterDelete = true
	testEtcd, err = createCluster(t, f, etcdClusterWithBackup(origEtcd, backupPolicy))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := deleteEtcdCluster(t, f, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	names, err = waitUntilSizeReached(t, f, testEtcd.Metadata.Name, 3, waitRestoreTimeout)
	if err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	pod, err = f.KubeClient.CoreV1().Pods(f.Namespace).Get(names[0], metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	checkEtcdData(t, fmt.Sprintf("http://%s:2379", pod.Status.PodIP))
}

func putDataToEtcd(url string) error {
	etcdcli, err := createEtcdClient(url)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	_, err = etcdcli.Put(ctx, etcdKeyFoo, etcdValBar)
	cancel()
	etcdcli.Close()
	return err
}

func checkEtcdData(t *testing.T, url string) {
	etcdcli, err := createEtcdClient(url)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.Get(ctx, etcdKeyFoo)
	cancel()
	etcdcli.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Kvs) != 1 {
		t.Errorf("want only 1 key result, get %d", len(resp.Kvs))
	} else {
		val := string(resp.Kvs[0].Value)
		if val != etcdValBar {
			t.Errorf("value want = '%s', get = '%s'", etcdValBar, val)
		}
	}
}

func calculateRestoreWaitTime(needDataClone bool) time.Duration {
	waitTime := 240 * time.Second
	if needDataClone {
		// Take additional time to clone the data.
		waitTime += 60 * time.Second
	}
	return waitTime
}

func testClusterRestoreS3SameName(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	testClusterRestoreWithBackupPolicy(t, false, newBackupPolicyS3())
}

func testClusterRestoreS3DifferentName(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	testClusterRestoreWithBackupPolicy(t, true, newBackupPolicyS3())
}
