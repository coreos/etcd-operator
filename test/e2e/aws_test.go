package e2e

import (
	"os"
	"testing"

	"github.com/coreos/etcd-operator/pkg/spec"
)

func TestS3MajorityDown(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	testDisasterRecoveryWithStorageType(t, 2, spec.BackupStorageTypeS3)
}

func TestS3AllDown(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	testDisasterRecoveryWithStorageType(t, 3, spec.BackupStorageTypeS3)
}

func TestClusterRestoreS3SameName(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	testClusterRestoreWithStorageType(t, true, spec.BackupStorageTypeS3)
}

func TestClusterRestoreS3DifferentName(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip("skipping test since AWS_TEST_ENABLED is not set.")
	}
	testClusterRestoreWithStorageType(t, false, spec.BackupStorageTypeS3)
}
