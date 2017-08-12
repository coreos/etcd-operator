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

package constants

import "time"

const (
	DefaultDialTimeout      = 5 * time.Second
	DefaultRequestTimeout   = 5 * time.Second
	DefaultSnapshotTimeout  = 1 * time.Minute
	DefaultSnapshotInterval = 1800 * time.Second

	DefaultBackupPodHTTPPort = 19999

	OperatorRoot   = "/var/tmp/etcd-operator"
	BackupMountDir = "/var/etcd-backup"

	PVProvisionerGCEPD  = "kubernetes.io/gce-pd"
	PVProvisionerAWSEBS = "kubernetes.io/aws-ebs"
	PVProvisionerCinder = "kubernetes.io/cinder"
	PVProvisionerNone   = "none"
)
