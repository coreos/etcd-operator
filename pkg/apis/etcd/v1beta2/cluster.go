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

import (
	"errors"
	"strings"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultBaseImage = "quay.io/coreos/etcd"
	defaultVersion   = "3.1.8"
)

var (
	// TODO: move validation code into separate package.
	ErrBackupUnsetRestoreSet = errors.New("spec: backup policy must be set if restore policy is set")
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EtcdClusterList is a list of etcd clusters.
type EtcdClusterList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdCluster `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EtcdCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterSpec   `json:"spec"`
	Status            ClusterStatus `json:"status"`
}

func (c *EtcdCluster) AsOwner() metav1.OwnerReference {
	trueVar := true
	return metav1.OwnerReference{
		APIVersion: SchemeGroupVersion.String(),
		Kind:       EtcdClusterResourceKind,
		Name:       c.Name,
		UID:        c.UID,
		Controller: &trueVar,
	}
}

type ClusterSpec struct {
	// Size is the expected size of the etcd cluster.
	// The etcd-operator will eventually make the size of the running
	// cluster equal to the expected size.
	// The vaild range of the size is from 1 to 7.
	Size int `json:"size"`

	// BaseImage is the base etcd image name that will be used to launch
	// etcd clusters. This is useful for private registries, etc.
	//
	// If image is not set, default is quay.io/coreos/etcd
	BaseImage string `json:"baseImage"`

	// Version is the expected version of the etcd cluster.
	// The etcd-operator will eventually make the etcd cluster version
	// equal to the expected version.
	//
	// The version must follow the [semver]( http://semver.org) format, for example "3.1.8".
	// Only etcd released versions are supported: https://github.com/coreos/etcd/releases
	//
	// If version is not set, default is "3.1.8".
	Version string `json:"version,omitempty"`

	// Paused is to pause the control of the operator for the etcd cluster.
	Paused bool `json:"paused,omitempty"`

	// Pod defines the policy to create pod for the etcd pod.
	//
	// Updating Pod does not take effect on any existing etcd pods.
	Pod *PodPolicy `json:"pod,omitempty"`

	// SelfHosted determines if the etcd cluster is used for a self-hosted
	// Kubernetes cluster.
	//
	// SelfHosted is a cluster initialization configuration. It cannot be updated.
	SelfHosted *SelfHostedPolicy `json:"selfHosted,omitempty"`

	// etcd cluster TLS configuration
	TLS *TLSPolicy `json:"TLS,omitempty"`
}

// PodPolicy defines the policy to create pod for the etcd container.
type PodPolicy struct {
	// Labels specifies the labels to attach to pods the operator creates for the
	// etcd cluster.
	// "app" and "etcd_*" labels are reserved for the internal use of the etcd operator.
	// Do not overwrite them.
	Labels map[string]string `json:"labels,omitempty"`

	// NodeSelector specifies a map of key-value pairs. For the pod to be eligible
	// to run on a node, the node must have each of the indicated key-value pairs as
	// labels.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// AntiAffinity determines if the etcd-operator tries to avoid putting
	// the etcd members in the same cluster onto the same node.
	AntiAffinity bool `json:"antiAffinity,omitempty"`

	// Resources is the resource requirements for the etcd container.
	// This field cannot be updated once the cluster is created.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// Tolerations specifies the pod's tolerations.
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`

	// List of environment variables to set in the etcd container.
	// This is used to configure etcd process. etcd cluster cannot be created, when
	// bad environement variables are provided. Do not overwrite any flags used to
	// bootstrap the cluster (for example `--initial-cluster` flag).
	// This field cannot be updated.
	EtcdEnv []v1.EnvVar `json:"etcdEnv,omitempty"`

	// By default, kubernetes will mount a service account token into the etcd pods.
	// AutomountServiceAccountToken indicates whether pods running with the service account should have an API token automatically mounted.
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty"`
}

func (c *ClusterSpec) Validate() error {
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return err
		}
	}

	if c.Pod != nil {
		for k := range c.Pod.Labels {
			if k == "app" || strings.HasPrefix(k, "etcd_") {
				return errors.New("spec: pod labels contains reserved label")
			}
		}
	}
	return nil
}

// Cleanup cleans up user passed spec, e.g. defaulting, transforming fields.
// TODO: move this to admission controller
func (c *ClusterSpec) Cleanup() {
	if len(c.BaseImage) == 0 {
		c.BaseImage = defaultBaseImage
	}

	if len(c.Version) == 0 {
		c.Version = defaultVersion
	}

	c.Version = strings.TrimLeft(c.Version, "v")
}
