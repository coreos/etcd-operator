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

package v1beta2

type BackupStorageType string

const (
	BackupStorageTypeS3 = "S3"

	AWSSecretCredentialsFileName = "credentials"
	AWSSecretConfigFileName      = "config"
)

// TODO: support per cluster S3 Source configuration.
type S3Source struct {
	// The name of the AWS S3 bucket to store backups in.
	//
	// S3Bucket overwrites the default etcd operator wide bucket.
	S3Bucket string `json:"s3Bucket,omitempty"`

	// Prefix is the S3 prefix used to prefix the bucket path.
	// It's the prefix at the beginning.
	// After that, it will have version and cluster specific paths.
	Prefix string `json:"prefix,omitempty"`

	// The name of the secret object that stores the AWS credential and config files.
	// The file name of the credential MUST be 'credentials'.
	// The file name of the config MUST be 'config'.
	// The profile to use in both files will be 'default'.
	//
	// AWSSecret overwrites the default etcd operator wide AWS credential and config.
	AWSSecret string `json:"awsSecret,omitempty"`
}
