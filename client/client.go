package client

import (
	"context"

	"github.com/coreos/etcd-operator/pkg/spec"
)

// Operator operators etcd clusters atop Kubernetes.
type Operator interface {
	// Create creates an etcd cluster with the given spec.
	Create(ctx context.Context, name string, spec *spec.ClusterSpec) error

	// Delete deletes the etcd cluster with given name
	Delete(ctx context.Context, name string) error

	// Update updates the etcd cluster with the given spec.
	Update(ctx context.Context, name string, spec *spec.ClusterSpec) error

	// Status gets the status of the etcd cluster with the given name.
	Status(ctx context.Context, name string) (error, *spec.ClusterStatus)
}
