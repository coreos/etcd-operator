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

type SelfHostedPolicy struct {
	// BootMemberClientEndpoint specifies a bootstrap member for the cluster.
	// If there is no bootstrap member, a completely new cluster will be created.
	// The boot member will be removed from the cluster once the self-hosted cluster
	// setup successfully.
	BootMemberClientEndpoint string `json:"bootMemberClientEndpoint,omitempty"`

	// SkipBootMemberRemoval specifies whether the removal of the bootstrap member
	// should be skipped. By default the operator will automatically remove the
	// bootstrap member from the new cluster - this happens during the pivot
	// procedure and is the first step of decommissioning the bootstrap member.
	// If unspecified, the default is `false`. If set to `true`, you are
	// expected to remove the boot member yourself from the etcd cluster.
	SkipBootMemberRemoval bool `json:"skipBootMemberRemoval,omitempty"`
}
