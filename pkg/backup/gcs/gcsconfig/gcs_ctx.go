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

// Package gcsconfig defines Google Cloud Storage configuration.
package gcsconfig

// GCSContext defines Google Cloud Storage top-level options.
type GCSContext struct {
	// GCSSecret is the key name for secret volume source.
	// The secret(JSON credential key) will be fetched with this key
	// from the Kubernetes client-go/SecretInterface.
	GCSSecret string
	// Bucket is the name of Google Cloud Storage bucket.
	// The supplied name must contain only lowercase letters, numbers, dashes,
	// underscores, and dots. See https://cloud.google.com/storage/docs/bucket-naming
	// for more detail.
	Bucket string
	// Scope defines permissions in Google Cloud Storage
	// (e.g. cloud.google.com/go/storage.ScopeFullControl,ScopeReadOnly,ScopeReadWrite).
	Scope string
}
