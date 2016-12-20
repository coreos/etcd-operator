package e2e

import (
	"os"
	"testing"

	"github.com/coreos/etcd-operator/pkg/spec"
)

func TestS3DisasterRecovery(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}

	t.Run("disaster recovery on S3", func(t *testing.T) {
		t.Run("2 members (majority) down", testS3MajorityDown)
		t.Run("3 members (all) down", testS3AllDown)
	})
}

func testS3MajorityDown(t *testing.T) {
	t.Parallel()
	testDisasterRecoveryWithStorageType(t, 2, spec.BackupStorageTypeS3)
}

func testS3AllDown(t *testing.T) {
	t.Parallel()
	testDisasterRecoveryWithStorageType(t, 3, spec.BackupStorageTypeS3)
}

func TestClusterRestoreS3(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}

	t.Run("restore cluster from backup on S3", func(t *testing.T) {
		t.Run("restore from the same name cluster", testClusterRestoreS3SameName)
		t.Run("restore from a different name cluster", testClusterRestoreS3DifferentName)
	})
}

func testClusterRestoreS3SameName(t *testing.T) {
	t.Parallel()
	testClusterRestoreWithStorageType(t, true, spec.BackupStorageTypeS3)
}

func testClusterRestoreS3DifferentName(t *testing.T) {
	t.Parallel()
	testClusterRestoreWithStorageType(t, false, spec.BackupStorageTypeS3)
}
