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

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s.io/client-go/pkg/api/meta/metatypes"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	defaultVersion = "3.1.2"

	TPRKind        = "cluster"
	TPRKindPlural  = "clusters"
	TPRGroup       = "etcd.coreos.com"
	TPRVersion     = "v1beta1"
	TPRDescription = "Managed etcd clusters"
)

var (
	ErrBackupUnsetRestoreSet = errors.New("spec: backup policy must be set if restore policy is set")
)

func TPRName() string {
	return fmt.Sprintf("%s.%s", TPRKind, TPRGroup)
}

type Cluster struct {
	unversioned.TypeMeta `json:",inline"`
	Metadata             v1.ObjectMeta `json:"metadata,omitempty"`
	Spec                 ClusterSpec   `json:"spec"`
	Status               ClusterStatus `json:"status"`
}

func (c *Cluster) AsOwner() metatypes.OwnerReference {
	trueVar := true
	// TODO: In 1.6 this is gonna be "k8s.io/kubernetes/pkg/apis/meta/v1"
	// Both api.OwnerReference and metatypes.OwnerReference are combined into that.
	return metatypes.OwnerReference{
		APIVersion: c.APIVersion,
		Kind:       c.Kind,
		Name:       c.Metadata.Name,
		UID:        c.Metadata.UID,
		Controller: &trueVar,
	}
}

type ClusterSpec struct {
	// Size is the expected size of the etcd cluster.
	// The etcd-operator will eventually make the size of the running
	// cluster equal to the expected size.
	// The vaild range of the size is from 1 to 7.
	Size int `json:"size"`

	// Version is the expected version of the etcd cluster.
	// The etcd-operator will eventually make the etcd cluster version
	// equal to the expected version.
	//
	// The version must follow the [semver]( http://semver.org) format, for example "3.1.2".
	// Only etcd released versions are supported: https://github.com/coreos/etcd/releases
	//
	// If version is not set, default is "3.1.2".
	Version string `json:"version"`

	// Paused is to pause the control of the operator for the etcd cluster.
	Paused bool `json:"paused,omitempty"`

	// Pod defines the policy to create pod for the etcd container.
	Pod *PodPolicy `json:"pod,omitempty"`

	// Backup defines the policy to backup data of etcd cluster if not nil.
	// If backup policy is set but restore policy not, and if a previous backup exists,
	// this cluster would face conflict and fail to start.
	Backup *BackupPolicy `json:"backup,omitempty"`

	// Restore defines the policy to restore cluster form existing backup if not nil.
	// It's not allowed if restore policy is set and backup policy not.
	Restore *RestorePolicy `json:"restore,omitempty"`

	// SelfHosted determines if the etcd cluster is used for a self-hosted
	// Kubernetes cluster.
	SelfHosted *SelfHostedPolicy `json:"selfHosted,omitempty"`

	// etcd cluster TLS configuration
	TLS *TLSPolicy `json:"TLS"`
}

// RestorePolicy defines the policy to restore cluster form existing backup if not nil.
type RestorePolicy struct {
	// BackupClusterName is the cluster name of the backup to recover from.
	BackupClusterName string `json:"backupClusterName"`

	// StorageType specifies the type of storage device to store backup files.
	// If not set, the default is "PersistentVolume".
	StorageType BackupStorageType `json:"storageType"`
}

// PodPolicy defines the policy to create pod for the etcd container.
type PodPolicy struct {
	// NodeSelector specifies a map of key-value pairs. For the pod to be eligible
	// to run on a node, the node must have each of the indicated key-value pairs as
	// labels.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// AntiAffinity determines if the etcd-operator tries to avoid putting
	// the etcd members in the same cluster onto the same node.
	AntiAffinity bool `json:"antiAffinity"`

	// Resources is the resource requirements for the etcd container.
	// This field cannot be updated once the cluster is created.
	Resources v1.ResourceRequirements `json:"resources"`
}

func (c *ClusterSpec) Validate() error {
	if c.Backup == nil && c.Restore != nil {
		return ErrBackupUnsetRestoreSet
	}
	if c.Backup != nil && c.Restore != nil {
		if c.Backup.StorageType != c.Restore.StorageType {
			return errors.New("spec: backup and restore storage types are different")
		}
	}
	return nil
}

// Cleanup cleans up user passed spec, e.g. defaulting, transforming fields.
// TODO: move this to admission controller
func (c *ClusterSpec) Cleanup() {
	if len(c.Version) == 0 {
		c.Version = defaultVersion
	}
	c.Version = strings.TrimLeft(c.Version, "v")
}

type ClusterPhase string

const (
	ClusterPhaseNone     ClusterPhase = ""
	ClusterPhaseCreating              = "Creating"
	ClusterPhaseRunning               = "Running"
	ClusterPhaseFailed                = "Failed"
)

type ClusterCondition struct {
	Type ClusterConditionType `json:"type"`

	Reason string `json:"reason"`

	TransitionTime time.Time `json:"transitionTime"`
}

type ClusterConditionType string

const (
	ClusterConditionReady = "Ready"

	ClusterConditionRemovingDeadMember = "RemovingDeadMember"

	ClusterConditionRecovering = "Recovering"

	ClusterConditionScalingUp   = "ScalingUp"
	ClusterConditionScalingDown = "ScalingDown"

	ClusterConditionUpgrading = "Upgrading"
)

type ClusterStatus struct {
	// Phase is the cluster running phase
	Phase  ClusterPhase `json:"phase"`
	Reason string       `json:"reason"`

	// ControlPuased indicates the operator pauses the control of the cluster.
	ControlPaused bool `json:"controlPaused"`

	// Condition keeps ten most recent cluster conditions
	Conditions []ClusterCondition `json:"conditions"`

	// Size is the current size of the cluster
	Size int `json:"size"`
	// Members are the etcd members in the cluster
	Members MembersStatus `json:"members"`
	// CurrentVersion is the current cluster version
	CurrentVersion string `json:"currentVersion"`
	// TargetVersion is the version the cluster upgrading to.
	// If the cluster is not upgrading, TargetVersion is empty.
	TargetVersion string `json:"targetVersion"`

	// BackupServiceStatus is the status of the backup service.
	// BackupServiceStatus only exists when backup is enabled in the
	// cluster spec.
	BackupServiceStatus *BackupServiceStatus `json:"backupServiceStatus,omitempty"`
}

type MembersStatus struct {
	// Ready are the etcd members that are ready to serve requests
	// The member names are the same as the etcd pod names
	Ready []string `json:"ready,omitempty"`
	// Unready are the etcd members not ready to serve requests
	Unready []string `json:"unready,omitempty"`
}

func (cs ClusterStatus) Copy() ClusterStatus {
	newCS := ClusterStatus{}
	b, err := json.Marshal(cs)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, &newCS)
	if err != nil {
		panic(err)
	}
	return newCS
}

func (cs *ClusterStatus) IsFailed() bool {
	if cs == nil {
		return false
	}
	return cs.Phase == ClusterPhaseFailed
}

func (cs *ClusterStatus) SetPhase(p ClusterPhase) {
	cs.Phase = p
}

func (cs *ClusterStatus) PauseControl() {
	cs.ControlPaused = true
}

func (cs *ClusterStatus) Control() {
	cs.ControlPaused = false
}

func (cs *ClusterStatus) UpgradeVersionTo(v string) {
	cs.TargetVersion = v
}

func (cs *ClusterStatus) SetVersion(v string) {
	cs.TargetVersion = ""
	cs.CurrentVersion = v
}

func (cs *ClusterStatus) SetReason(r string) {
	cs.Reason = r
}

func (cs *ClusterStatus) AppendScalingUpCondition(from, to int) {
	c := ClusterCondition{
		Type:           ClusterConditionScalingUp,
		Reason:         scalingReason(from, to),
		TransitionTime: time.Now(),
	}
	cs.appendCondition(c)
}

func (cs *ClusterStatus) AppendScalingDownCondition(from, to int) {
	c := ClusterCondition{
		Type:           ClusterConditionScalingDown,
		Reason:         scalingReason(from, to),
		TransitionTime: time.Now(),
	}
	cs.appendCondition(c)
}

func (cs *ClusterStatus) AppendRecoveringCondition() {
	c := ClusterCondition{
		Type:           ClusterConditionRecovering,
		TransitionTime: time.Now(),
	}
	cs.appendCondition(c)
}

func (cs *ClusterStatus) AppendUpgradingCondition(to string, member string) {
	reason := fmt.Sprintf("upgrading cluster member %s version to %v", member, to)

	c := ClusterCondition{
		Type:           ClusterConditionUpgrading,
		Reason:         reason,
		TransitionTime: time.Now(),
	}
	cs.appendCondition(c)
}

func (cs *ClusterStatus) AppendRemovingDeadMember(name string) {
	reason := fmt.Sprintf("removing dead member %s", name)

	c := ClusterCondition{
		Type:           ClusterConditionRemovingDeadMember,
		Reason:         reason,
		TransitionTime: time.Now(),
	}
	cs.appendCondition(c)
}

func (cs *ClusterStatus) SetReadyCondition() {
	c := ClusterCondition{
		Type:           ClusterConditionReady,
		TransitionTime: time.Now(),
	}

	if len(cs.Conditions) == 0 {
		cs.appendCondition(c)
		return
	}

	lastc := cs.Conditions[len(cs.Conditions)-1]
	if lastc.Type == ClusterConditionReady {
		return
	}
	cs.appendCondition(c)
}

func (cs *ClusterStatus) appendCondition(c ClusterCondition) {
	cs.Conditions = append(cs.Conditions, c)
	if len(cs.Conditions) > 10 {
		cs.Conditions = cs.Conditions[1:]
	}
}

func scalingReason(from, to int) string {
	return fmt.Sprintf("Current cluster size: %d, desired cluster size: %d", from, to)
}
