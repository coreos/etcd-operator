package spec

import (
	"errors"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
)

var (
	ErrBackupUnsetRestoreSet = errors.New("spec: backup policy must be set if restore policy is set")
)

// TiDBCluster is the controll script's spec
type TiDBCluster struct {
	unversioned.TypeMeta `json:",inline"`
	api.ObjectMeta       `json:"metadata,omitempty"`
	Spec                 ClusterSpec `json:"spec"`
}

// ServiceSpec is a simple service template, we also can split it into tidb/pd/tikv while we need
type ServiceSpec struct {
	Size         int               `json:"size"`
	Version      string            `json:"version"`
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	AntiAffinity bool              `json:"antiAffinity"`
}

// ClusterSpec contains details of tidb-cluster spec
type ClusterSpec struct {
	PD     *ServiceSpec `json:"pd,omitempty"`
	TIDB   *ServiceSpec `json:"tidb,omitempty"`
	TiKV   *ServiceSpec `json:"tikv,omitempty"`
	Paused bool         `json:"paused,omitempty"`
}
