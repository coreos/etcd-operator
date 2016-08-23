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
}
