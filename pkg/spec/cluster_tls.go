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

package spec

// TLSPolicy defines the TLS policy of an etcd cluster
type TLSPolicy struct {
	// StaticTLS enables user to generate static x509 certificates and keys,
	// put them into Kubernetes secrets, and specify them into here.
	Static *StaticTLS `json:"static"`
}

type StaticTLS struct {
	// Member contains secrets containing TLS certs used by each etcd member pod.
	Member *MemberSecret `json:"member"`
	// OperatorSecret is the secret containing TLS certs used by operator to
	// talk securely to this cluster.
	OperatorSecret string `json:"operatorSecret"`
}

type MemberSecret struct {
	// PeerSecret is the secret containing TLS certs used by each etcd member pod
	// for the communication between etcd peers.
	PeerSecret string `json:"peerSecret"`
	// ClientSecret is the secret containing TLS certs used by each etcd member pod
	// for the communication between etcd and its clients.
	ClientSecret string `json:"clientSecret"`
}
