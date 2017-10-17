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

package env

const (
	ClusterSpec       = "CLUSTER_SPEC"
	BackupSpec        = "BACKUP_SPEC"
	AWSS3Bucket       = "AWS_S3_BUCKET"
	AWSConfig         = "AWS_CONFIG_FILE"
	ABSContainer      = "AZURE_STORAGE_CONTAINER"
	ABSStorageAccount = "AZURE_STORAGE_ACCOUNT"
	ABSStorageKey     = "AZURE_STORAGE_KEY"
)
