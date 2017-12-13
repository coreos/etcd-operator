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
	"testing"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"
	ini "gopkg.in/ini.v1"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TestBackupAndRestoreOnS3CompatibleStorage test the backup on a S3 compatible storage
func TestBackupAndRestoreOnS3CompatibleStorage(t *testing.T) {

	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}

	setupMinio(t)

	s3Path := testEtcdBackupOperatorForS3CompatibleStorageBackup(t)
	if len(s3Path) == 0 {
		t.Fatal("skipping restore test: S3 path not set despite testEtcdBackupOperatorForS3Backup success")
	}
	testEtcdRestoreOperatorForS3CompatibleStorageSource(t, s3Path)

	teardownMinio(t)
}

// testEtcdBackupOperatorForS3CompatibleStorageBackup tests if etcd backup operator can save etcd backup to provided S3 endpoint.
// It returns the full S3 path where the backup is saved.
func testEtcdBackupOperatorForS3CompatibleStorageBackup(t *testing.T) string {
	f := framework.Global
	suffix := fmt.Sprintf("-%d", rand.Uint64())
	clusterName := "tls-test" + suffix
	memberPeerTLSSecret := "etcd-peer-tls" + suffix
	memberClientTLSSecret := "etcd-server-tls" + suffix
	operatorClientTLSSecret := "etcd-client-tls" + suffix

	err := e2eutil.PrepareTLS(clusterName, f.Namespace, memberPeerTLSSecret, memberClientTLSSecret, operatorClientTLSSecret)
	if err != nil {
		t.Fatal(err)
	}

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
	backCR.Spec.S3.AWSEndpoint = "http://minio:9000"

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
	s3cli, err := s3factory.NewClientFromSecret(f.KubeClient, f.Namespace, "http://minio:9000", os.Getenv("TEST_AWS_SECRET"))

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

// testEtcdRestoreOperatorForS3CompatibleStorageSource tests if the restore-operator can restore an etcd cluster from an S3 restore source
func testEtcdRestoreOperatorForS3CompatibleStorageSource(t *testing.T, s3Path string) {
	f := framework.Global

	restoreSource := api.RestoreSource{S3: e2eutil.NewS3RestoreSource(s3Path, os.Getenv("TEST_AWS_SECRET"))}
	restoreSource.S3.AWSEndpoint = "http://minio:9000"

	er := e2eutil.NewEtcdRestore("test-etcd-restore-", "3.2.10", 3, restoreSource)
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

// setupMinio sets up a standalone minio server
func setupMinio(t *testing.T) {

	f := framework.Global

	// Get S3 secrets and provide it to minio server
	secret, err := f.KubeClient.Core().Secrets(f.Namespace).Get(os.Getenv("TEST_AWS_SECRET"), metav1.GetOptions{})

	if err != nil {
		t.Fatal("an error occurred while reading aws secret")
	}

	credsini := secret.Data[api.AWSSecretCredentialsFileName]

	// Read AWS configuration file
	cfg, err := ini.Load(credsini)

	// Extract keys
	accessKey := cfg.Section("default").Key("aws_access_key_id").String()
	secretAccessKey := cfg.Section("default").Key("aws_secret_access_key").String()

	// Minio pod
	minioPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "minio",
			Labels: map[string]string{
				"app": "minio",
			},
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "storage",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
			InitContainers: []v1.Container{
				{
					Name:  "create-bucket",
					Image: "busybox",
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "storage",
							MountPath: "/storage",
						},
					},
					Command: []string{
						"mkdir",
						"-p",
						fmt.Sprintf("%s/%s", "/storage", os.Getenv("TEST_S3_BUCKET")),
					},
				},
			},
			Containers: []v1.Container{
				{
					Name:            "minio",
					Image:           "minio/minio",
					ImagePullPolicy: v1.PullIfNotPresent,
					Args: []string{
						"server",
						"/storage",
					},
					Env: []v1.EnvVar{
						{
							Name:  "MINIO_ACCESS_KEY",
							Value: accessKey,
						},
						{
							Name:  "MINIO_SECRET_KEY",
							Value: secretAccessKey,
						},
					},
					Ports: []v1.ContainerPort{
						{
							ContainerPort: 9000,
						},
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "storage",
							MountPath: "/storage",
						},
					},
				},
			},
		},
	}

	// Minio service
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "minio",
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"app": "minio",
			},
			Ports: []v1.ServicePort{
				{
					Port:       9000,
					TargetPort: intstr.FromInt(9000),
				},
			},
		},
	}

	f.KubeClient.Core().Services(f.Namespace).Create(svc)

	_, err = f.KubeClient.Core().Pods(f.Namespace).Create(minioPod)
	if err != nil {
		t.Fatal("an error occurred while running minio pod")
	}

	// Wait for minio pod
	interval := 5 * time.Second
	err = retryutil.Retry(interval, int(30*time.Second/(interval)), func() (bool, error) {
		pod, err := f.KubeClient.Core().Pods(f.Namespace).Get(minioPod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if pod.Status.Phase == v1.PodRunning {
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		t.Fatalf("failed to wait minio pods running: %v", err)
	}

}

// teardownMinio deletes minio server pod
func teardownMinio(t *testing.T) {
	f := framework.Global
	if err := f.KubeClient.Core().Pods(f.Namespace).Delete("minio", &metav1.DeleteOptions{}); err != nil {
		t.Fatalf("failed to delete minio pod: %v", err)
	}
}
