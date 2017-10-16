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

package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	backupenv "github.com/coreos/etcd-operator/pkg/backup/env"
)

const (
	testClusterName    = "test_cluster"
	testBucket         = "test_bucket"
	testSecret         = "test_secret"
	testOperatorSecret = "test_operator_secret"
	testPeerSecret     = "test_peer_secret"
	testServerSecret   = "test_server_secret"
)

// TestParsingSpecsFromEnv ensures ParsingSpecsFromEnv functions parses
// specs fro env correctly.
func TestParsingSpecsFromEnv(t *testing.T) {
	tests := []struct {
		// inputs
		clusterSpec *api.ClusterSpec
		backupSpec  *api.BackupSpec

		// expected outputs
		expectBackupPolicy *api.BackupPolicy
		expectTLS          *api.TLSPolicy
		hasError           bool
	}{
		{
			// empty specs
			hasError: true,
		},
		{
			// no tls no backup policy.
			clusterSpec: newClusterSpec(false, false),

			// expect error on no backup policy found.
			expectBackupPolicy: nil,
			expectTLS:          nil,
			hasError:           true,
		},
		{
			// no tls but has backup policy.
			clusterSpec: newClusterSpec(false, true),

			expectBackupPolicy: newS3BackupPolicy(),
			expectTLS:          nil,
			hasError:           false,
		},
		{
			// has tls and has backup policy.
			clusterSpec: newClusterSpec(true, true),

			expectBackupPolicy: newS3BackupPolicy(),
			expectTLS:          newTLS(),
			hasError:           false,
		},
		{
			// no cluster spec but has backup spec.
			backupSpec: newBackupSpec(),

			// expect error on no cluster spec found.
			expectBackupPolicy: nil,
			expectTLS:          nil,
			hasError:           true,
		},
		{
			// no tls no backup policy but has backup spec.
			clusterSpec: newClusterSpec(false, false),
			backupSpec:  newBackupSpec(),

			// expect a backup policy created from the passed-in backup spec.
			expectBackupPolicy: backupPolicyFromBackupSpec(),
			expectTLS:          nil,
			hasError:           false,
		},
		{
			// no backup policy but has tls and backup spec.
			clusterSpec: newClusterSpec(true, false),
			backupSpec:  newBackupSpec(),

			// expect a backup policy created from the passed-in backup spec.
			expectBackupPolicy: backupPolicyFromBackupSpec(),
			expectTLS:          newTLS(),
			hasError:           false,
		},
		{
			// has tls, backup policy, and backup spec.
			clusterSpec: newClusterSpec(true, true),
			backupSpec:  newBackupSpec(),

			// expect the backup policy is from the passed-in backup spec not the policy from cluster spec.
			expectBackupPolicy: backupPolicyFromBackupSpec(),
			expectTLS:          newTLS(),
			hasError:           false,
		},
	}

	for i, tt := range tests {
		t.Logf("%v: test case %+v ", i, tt)
		if tt.clusterSpec != nil {
			mcs, err := json.Marshal(tt.clusterSpec)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Setenv(backupenv.ClusterSpec, string(mcs))
			if err != nil {
				t.Fatal(err)
			}
		}

		if tt.backupSpec != nil {
			mbs, err := json.Marshal(tt.backupSpec)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Setenv(backupenv.BackupSpec, string(mbs))
			if err != nil {
				t.Fatal(err)
			}
		}

		bp, tls, err := parseSpecsFromEnv()
		if tt.hasError != (err != nil) {
			t.Fatalf("expect having error=%v, but got (%v)", tt.hasError, err)
		}

		if !reflect.DeepEqual(tt.expectBackupPolicy, bp) {
			t.Fatalf("expect backup policy %+v, but got %+v", tt.expectBackupPolicy, bp)
		}

		if !reflect.DeepEqual(tt.expectTLS, tls) {
			t.Fatalf("expect tls policy %+v, but got %+v", tt.expectTLS, tls)
		}
		if err = os.Unsetenv(backupenv.ClusterSpec); err != nil {
			t.Fatal(err)
		}
		if err = os.Unsetenv(backupenv.BackupSpec); err != nil {
			t.Fatal(err)
		}
	}
}

func newClusterSpec(tls, backpolicy bool) *api.ClusterSpec {
	ec := &api.ClusterSpec{}
	if tls {
		ec.TLS = newTLS()
	}
	if backpolicy {
		ec.Backup = newS3BackupPolicy()
	}
	return ec
}

func newTLS() *api.TLSPolicy {
	return &api.TLSPolicy{
		Static: &api.StaticTLS{
			Member: &api.MemberSecret{
				PeerSecret:   testPeerSecret,
				ServerSecret: testServerSecret,
			},
			OperatorSecret: testOperatorSecret,
		},
	}
}

func newS3BackupPolicy() *api.BackupPolicy {
	return &api.BackupPolicy{
		BackupIntervalInSecond: 60 * 60,
		MaxBackups:             5,
		StorageType:            api.BackupStorageTypeS3,
		StorageSource: api.StorageSource{
			S3: newS3Source(),
		},
	}
}

// backupPolicyFromBackupSpec returns a BackupPolicy that represents
// the BackupSpec returned by newBackupSpec()
func backupPolicyFromBackupSpec() *api.BackupPolicy {
	return &api.BackupPolicy{
		StorageType: api.BackupStorageTypeS3,
		StorageSource: api.StorageSource{
			S3: newS3Source(),
		},
	}
}

func newBackupSpec() *api.BackupSpec {
	return &api.BackupSpec{
		ClusterName: testClusterName,
		StorageType: api.BackupStorageTypeS3,
		BackupStorageSource: api.BackupStorageSource{
			S3: newS3Source(),
		},
	}
}

func newS3Source() *api.S3Source {
	return &api.S3Source{
		S3Bucket:  testBucket,
		AWSSecret: testSecret,
	}
}
