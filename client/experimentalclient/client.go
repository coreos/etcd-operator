package experimentalclient

import (
	"context"
	"net/http"

	"github.com/coreos/etcd-operator/pkg/spec"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/runtime/serializer"
	"k8s.io/client-go/1.5/rest"
)

// NOTE: This is experimental client. We will likely change it in the future
// Operator operates etcd clusters atop Kubernetes.
type Operator interface {
	// Create creates an etcd cluster.
	Create(ctx context.Context, name string, spec spec.ClusterSpec) error

	// Delete deletes the etcd cluster.
	Delete(ctx context.Context, name string) error

	// Update updates the etcd cluster with the given spec.
	Update(ctx context.Context, name string, spec spec.ClusterSpec) error

	// Get gets the etcd cluster information.
	Get(ctx context.Context, name string) (*spec.Cluster, error)

	// List lists all etcd clusters.
	List(ctx context.Context) (*spec.ClusterList, error)
}

var (
	groupversion = unversioned.GroupVersion{
		Group:   spec.TPRGroup,
		Version: spec.TPRVersion,
	}
)

func init() {
	// FIXME: if we add this scheme, Kubernetes client will not work
	// correctly. Fix it before use operator trp client.
	needfix := true
	if needfix {
		return
	}

	schemeBuilder := runtime.NewSchemeBuilder(
		func(scheme *runtime.Scheme) error {
			scheme.AddKnownTypes(
				groupversion,
				&spec.Cluster{},
				&spec.ClusterList{},
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
		tprName:   spec.TPRName(),
		ns:        namespace,
	}, nil

}

func (o *operator) Create(ctx context.Context, name string, cspec spec.ClusterSpec) error {
	cluster := &spec.Cluster{
		Metadata: v1.ObjectMeta{
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

func (o *operator) Get(ctx context.Context, name string) (*spec.Cluster, error) {
	cluster := &spec.Cluster{}

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

func (o *operator) List(ctx context.Context) (*spec.ClusterList, error) {
	clusters := &spec.ClusterList{}

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
