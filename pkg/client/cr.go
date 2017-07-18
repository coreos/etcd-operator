// Copyright 2017 The etcd-operator Authors
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

package client

import (
	"context"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

type EtcdClusterCR interface {
	// Create creates an etcd cluster CR with the desired CR
	Create(ctx context.Context, namespace string, cl *spec.EtcdCluster) (*spec.EtcdCluster, error)

	// Get returns the specified etcd cluster CR
	Get(ctx context.Context, name, namespace string) (*spec.EtcdCluster, error)

	// Delete deletes the specified etcd cluster CR
	Delete(ctx context.Context, name, namespace string) error

	// Update updates the etcd cluster CR.
	Update(ctx context.Context, name, namespace string, etcdCluster *spec.EtcdCluster) (*spec.EtcdCluster, error)
}

type etcdClusterCR struct {
	client     *rest.RESTClient
	crScheme   *runtime.Scheme
	paramCodec runtime.ParameterCodec
}

func MustNewCRInCluster() (EtcdClusterCR, error) {
	cfg, err := k8sutil.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return NewCRClient(cfg)
}

func NewCRClient(cfg *rest.Config) (EtcdClusterCR, error) {
	cli, crScheme, err := New(cfg)
	if err != nil {
		return nil, err
	}
	return &etcdClusterCR{
		client:     cli,
		crScheme:   crScheme,
		paramCodec: runtime.NewParameterCodec(crScheme),
	}, nil
}

// TODO: make this private so that we don't expose RESTClient once operator code uses this client instead of REST calls
func New(cfg *rest.Config) (*rest.RESTClient, *runtime.Scheme, error) {
	crScheme := runtime.NewScheme()
	if err := spec.AddToScheme(crScheme); err != nil {
		return nil, nil, err
	}

	config := *cfg
	config.GroupVersion = &spec.SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(crScheme)}

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, nil, err
	}

	return client, crScheme, nil
}

func (c *etcdClusterCR) Create(ctx context.Context, namespace string, etcdCluster *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	result := &spec.EtcdCluster{}
	err := c.client.Post().Context(ctx).
		Namespace(namespace).
		Resource(spec.CRDResourcePlural).
		Body(etcdCluster).
		Do().
		Into(result)
	return result, err
}

func (c *etcdClusterCR) Get(ctx context.Context, name, namespace string) (*spec.EtcdCluster, error) {
	result := &spec.EtcdCluster{}
	err := c.client.Get().Context(ctx).
		Namespace(namespace).
		Resource(spec.CRDResourcePlural).
		Name(name).
		Do().
		Into(result)
	return result, err
}

func (c *etcdClusterCR) Delete(ctx context.Context, name, namespace string) error {
	return c.client.Delete().Context(ctx).
		Namespace(namespace).
		Resource(spec.CRDResourcePlural).
		Name(name).
		Do().
		Error()
}

func (c *etcdClusterCR) Update(ctx context.Context, name, namespace string, etcdCluster *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	result := &spec.EtcdCluster{}
	err := c.client.Put().Context(ctx).
		Namespace(namespace).
		Resource(spec.CRDResourcePlural).
		Name(name).
		Body(etcdCluster).
		Do().
		Into(result)
	return result, err
}
