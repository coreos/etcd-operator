package e2e

import (
	"os"
	"testing"

	"github.com/coreos/etcd-operator/pkg/spec"
)

func TestS3MajorityDown(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip()
	}
	testDisasterRecoveryWithStorageType(t, 2, spec.BackupStorageTypeS3)
}

func TestS3AllDown(t *testing.T) {
	if os.Getenv("AWS_TEST_ENABLED") != "true" {
		t.Skip()
	}
	testDisasterRecoveryWithStorageType(t, 3, spec.BackupStorageTypeS3)
}
