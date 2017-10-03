package e2e

import (
	"os"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	backups3 "github.com/coreos/etcd-operator/pkg/backup/s3"
	"github.com/coreos/etcd-operator/test/e2e/e2eutil"
	"github.com/coreos/etcd-operator/test/e2e/framework"
)

// TestEtcdBackupOperatorForS3Backup tests if etcd backup operator can save etcd backup to S3.
func TestEtcdBackupOperatorForS3Backup(t *testing.T) {
	if os.Getenv(envParallelTest) == envParallelTestTrue {
		t.Parallel()
	}
	f := framework.Global
	err := f.SetupEtcdBackupOperator()
	if err != nil {
		t.Fatalf("fail to create etcd backup operator: %v", err)
	}
	defer func() {
		err = f.DeleteEtcdBackupOperator()
		if err != nil {
			t.Fatalf("fail to deletes etcd backup operator: %v", err)
		}
	}()

	testEtcd, err := e2eutil.CreateCluster(t, f.CRClient, f.Namespace, e2eutil.NewCluster("test-etcd-", 3))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := e2eutil.DeleteCluster(t, f.CRClient, f.KubeClient, testEtcd); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := e2eutil.WaitUntilSizeReached(t, f.CRClient, 3, 6, testEtcd); err != nil {
		t.Fatalf("failed to create 3 members etcd cluster: %v", err)
	}

	if err := f.SetupEtcdBackupOperator(); err != nil {
		t.Fatalf("failed to etcd backup operator: %v", err)
	}

	defer func() {
		if err := f.DeleteEtcdBackupOperator(); err != nil {
			t.Fatalf("failed to delete etcd backup operator: %v", err)
		}
	}()

	eb, err := e2eutil.CreateBackupCR(t, f.CRClient, f.Namespace, e2eutil.NewS3Backup(testEtcd.Name))
	if err != nil {
		t.Fatalf("failed to create etcd backup cr: %v", err)
	}
	defer func() {
		if err := e2eutil.DeleteBackupCR(t, f.CRClient, f.KubeClient, eb); err != nil {
			t.Fatalf("failed to delete etcd backup cr: %v", err)
		}
	}()
	s3Spec := eb.Spec.S3
	prefix := backupapi.ToS3Prefix(s3Spec.Prefix, eb.Namespace, eb.Name)
	s3cli := backups3.NewFromClient(s3Spec.S3Bucket, prefix, f.S3Cli)
	// TODO check CR status for backup
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		keys, err := s3cli.List()
		if err != nil {
			t.Fatalf("failed to list s3 objects: %v", err)
		}
		if len(keys) == 0 {
			// retry if no backup found.
			continue
		}
		if len(keys) > 1 {
			t.Fatalf("expected 1 backup, but got %v", len(keys))
		}
		break
	}
}
