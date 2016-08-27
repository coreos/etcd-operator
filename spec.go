package main

import (
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
)

type EtcdClusterList struct {
	unversioned.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	unversioned.ListMeta `json:"metadata,omitempty"`
	// Items is a list of third party objects
	Items []EtcdCluster `json:"items"`
}

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
	// Backup is the backup strategy for the etcd cluster.
	// There is no backup by default.
	Backup Backup `json:"backup"`
}

type Backup struct {
	// MaxSnapshot is the maximum number of snapshot files to retain. 0 is disable backup.
	// If backup is disabled, the etcd cluster cannot recover from a
	// disaster failure (lose more than half of its members at the same
	// time).
	MaxSnapshot int
	// RequiredVolumeSizeInMB specifies the required volume size to perform backups.
	// Controller will claim the required size before creating the etcd cluster for backup
	// purpose.
	// If the snapshot size is larger than the size specified, backup fails.
	RequiredVolumeSizeInMB int
}
