package client

import (
	"context"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

type BackupCR interface {
	RESTClient() *rest.RESTClient

	Get(ctx context.Context, namespace, name string) (*api.EtcdBackup, error)
	Create(ctx context.Context, vault *api.EtcdBackup) (*api.EtcdBackup, error)
	Delete(ctx context.Context, namespace, name string) error
	Update(ctx context.Context, vault *api.EtcdBackup) (*api.EtcdBackup, error)
}

func MustNewInCluster() BackupCR {
	cfg, err := k8sutil.InClusterConfig()
	if err != nil {
		panic(err)
	}
	cli, err := NewCRClient(cfg)
	if err != nil {
		panic(err)
	}
	return cli
}

func NewCRClient(cfg *rest.Config) (BackupCR, error) {
	cli, crScheme, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return &backupCR{
		restCli:    cli,
		crScheme:   crScheme,
		paramCodec: runtime.NewParameterCodec(crScheme),
	}, nil
}

func newClient(cfg *rest.Config) (*rest.RESTClient, *runtime.Scheme, error) {
	crScheme := runtime.NewScheme()
	if err := api.AddToScheme(crScheme); err != nil {
		return nil, nil, err
	}

	config := *cfg
	config.GroupVersion = &api.SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(crScheme)}

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, nil, err
	}

	return client, crScheme, nil
}

type backupCR struct {
	restCli    *rest.RESTClient
	crScheme   *runtime.Scheme
	paramCodec runtime.ParameterCodec
}

func (bcr *backupCR) RESTClient() *rest.RESTClient {
	return bcr.restCli
}

func (bcr *backupCR) Get(ctx context.Context, namespace, name string) (*api.EtcdBackup, error) {
	eb := &api.EtcdBackup{}
	err := bcr.restCli.Get().Context(ctx).
		Namespace(namespace).
		Resource(api.CRDResourcePlural).
		Name(name).
		Do().
		Into(eb)
	return eb, err
}

func (bcr *backupCR) Create(ctx context.Context, backup *api.EtcdBackup) (*api.EtcdBackup, error) {
	eb := &api.EtcdBackup{}
	err := bcr.restCli.Post().Context(ctx).
		Namespace(backup.Namespace).
		Resource(api.CRDResourcePlural).
		Body(backup).
		Do().
		Into(eb)
	return eb, err
}

func (bcr *backupCR) Delete(ctx context.Context, namespace, name string) error {
	return bcr.restCli.Delete().Context(ctx).
		Namespace(namespace).
		Resource(api.CRDResourcePlural).
		Name(name).
		Do().
		Error()
}

func (bcr *backupCR) Update(ctx context.Context, backup *api.EtcdBackup) (*api.EtcdBackup, error) {
	eb := &api.EtcdBackup{}
	err := bcr.restCli.Put().Context(ctx).
		Namespace(backup.Namespace).
		Resource(api.CRDResourcePlural).
		Name(backup.Name).
		Body(backup).
		Do().
		Into(eb)
	return eb, err
}
