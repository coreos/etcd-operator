package controller

import (
	"github.com/coreos/kube-etcd-controller/pkg/cluster"
	"k8s.io/kubernetes/pkg/api/unversioned"
)

type EtcdClusterList struct {
	unversioned.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	unversioned.ListMeta `json:"metadata,omitempty"`
	// Items is a list of third party objects
	Items []cluster.EtcdCluster `json:"items"`
}
