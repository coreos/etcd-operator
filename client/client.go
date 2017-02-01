package client

import (
	"context"
	"net/http"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/runtime/serializer"
	"k8s.io/client-go/1.5/rest"
)

// Operator operators etcd clusters atop Kubernetes.
type Operator interface {
	// Create creates an etcd cluster.
	Create(ctx context.Context, name string, spec spec.ClusterSpec) error

	// Delete deletes the etcd cluster.
	Delete(ctx context.Context, name string) error

	// Update updates the etcd cluster with the given spec.
	Update(ctx context.Context, name string, spec spec.ClusterSpec) error

	// Get gets the etcd cluster information.
	Get(ctx context.Context, name string) (*spec.EtcdCluster, error)

	// List lists all etcd clusters.
	List(ctx context.Context) (*spec.EtcdClusterList, error)
}

var (
	groupversion = unversioned.GroupVersion{
		Group:   "coreos.com",
		Version: "v1",
	}
)

func init() {
	schemeBuilder := runtime.NewSchemeBuilder(
		func(scheme *runtime.Scheme) error {
			scheme.AddKnownTypes(
				groupversion,
				&spec.EtcdCluster{},
				&spec.EtcdClusterList{},
				&v1.ListOptions{},
				&v1.DeleteOptions{},
			)
			return nil
		})
	schemeBuilder.AddToScheme(api.Scheme)
}

type operator struct {
	tprClient *rest.RESTClient
	tprName   string
	ns        string
}

func NewOperator(namespace string) (Operator, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	configureClient(cfg)

	tprclient, err := rest.RESTClientFor(cfg)
	if err != nil {
		return nil, err
	}

	return &operator{
		tprClient: tprclient,
		tprName:   constants.TPRName,
		ns:        namespace,
	}, nil

}

func (o *operator) Create(ctx context.Context, name string, cspec spec.ClusterSpec) error {
	cluster := &spec.EtcdCluster{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		Spec: cspec,
	}

	err := o.tprClient.Post().
		Resource(o.tprName).
		Namespace(o.ns).
		Body(cluster).
		Do().Error()

	return err
}

func (o *operator) Delete(ctx context.Context, name string) error {
	return o.tprClient.Delete().
		Resource(o.tprName).
		Namespace(o.ns).Name(name).Do().Error()
}

func (o *operator) Update(ctx context.Context, name string, spec spec.ClusterSpec) error {
	for {
		e, err := o.Get(ctx, name)
		if err != nil {
			return err
		}

		e.Spec = spec
		var statusCode int

		err = o.tprClient.Put().
			Resource(o.tprName).
			Namespace(o.ns).
			Name(name).
			Body(e).
			Do().StatusCode(&statusCode).Error()

		if statusCode == http.StatusConflict {
			continue
		}

		return err
	}
}

func (o *operator) Get(ctx context.Context, name string) (*spec.EtcdCluster, error) {
	cluster := &spec.EtcdCluster{}

	err := o.tprClient.Get().
		Resource(o.tprName).
		Namespace(o.ns).
		Name(name).
		Do().Into(cluster)

	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func (o *operator) List(ctx context.Context) (*spec.EtcdClusterList, error) {
	clusters := &spec.EtcdClusterList{}

	err := o.tprClient.Get().
		Resource(o.tprName).
		Namespace(o.ns).
		Do().Into(clusters)

	if err != nil {
		return nil, err
	}

	return clusters, nil
}

func configureClient(config *rest.Config) {
	config.GroupVersion = &groupversion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}
}
