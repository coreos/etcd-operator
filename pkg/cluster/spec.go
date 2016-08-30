package cluster

import (
	"github.com/coreos/kube-etcd-controller/pkg/backup"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
)

type EtcdCluster struct {
	unversioned.TypeMeta `json:",inline"`
	api.ObjectMeta       `json:"metadata,omitempty"`
	Spec                 Spec `json: "spec"`
}

type Spec struct {
	// Size is the expected size of the etcd cluster.
	// The controller will eventually make the size of the running
	// cluster equal to the expected size.
	// The vaild range of the size is from 1 to 7.
	Size int `json:"size"`
	// AntiAffinity determines if the controller tries to avoid putting
	// the etcd members in the same cluster onto the same node.
	AntiAffinity bool `json:"antiAffinity"`
	// Version is the expected version of the etcd cluster.
	// The controller will eventually make the etcd cluster version
	// equal to the expected version.
	Version string `json:"version"`
	// Backup is the backup policy for the etcd cluster.
	// There is no backup by default.
	Backup *backup.Policy `json:"backup,omitempty"`
}
