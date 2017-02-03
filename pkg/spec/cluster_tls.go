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

package spec

type ClusterTLSPolicy struct {
	Static StaticTLS `json:"static"`
	//TODO: add more TLS options here
}

type StaticTLS struct {
	// Secret containing peer and client CA cert files
	CASecretName string `json:"caSecretName"`
	// Secret containing peer-interface and client-interface server x509 key/cert
	// Using this approach means all nodes have the same "identity", which is less than ideal
	NodeSecretName string `json:"nodeSecretName"`
	// Secret containing etcd client key/cert
	ClientSecretName string `json:"clientSecretName"`
}
