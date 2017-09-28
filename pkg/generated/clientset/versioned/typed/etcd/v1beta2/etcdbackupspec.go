/*
Copyright 2017 The etcd-operator Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package v1beta2

import (
	v1beta2 "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	scheme "github.com/coreos/etcd-operator/pkg/generated/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// EtcdBackupSpecsGetter has a method to return a EtcdBackupSpecInterface.
// A group's client should implement this interface.
type EtcdBackupSpecsGetter interface {
	EtcdBackupSpecs(namespace string) EtcdBackupSpecInterface
}

// EtcdBackupSpecInterface has methods to work with EtcdBackupSpec resources.
type EtcdBackupSpecInterface interface {
	Create(*v1beta2.EtcdBackupSpec) (*v1beta2.EtcdBackupSpec, error)
	Update(*v1beta2.EtcdBackupSpec) (*v1beta2.EtcdBackupSpec, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1beta2.EtcdBackupSpec, error)
	List(opts v1.ListOptions) (*v1beta2.EtcdBackupSpecList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta2.EtcdBackupSpec, err error)
	EtcdBackupSpecExpansion
}

// etcdBackupSpecs implements EtcdBackupSpecInterface
type etcdBackupSpecs struct {
	client rest.Interface
	ns     string
}

// newEtcdBackupSpecs returns a EtcdBackupSpecs
func newEtcdBackupSpecs(c *EtcdV1beta2Client, namespace string) *etcdBackupSpecs {
	return &etcdBackupSpecs{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the etcdBackupSpec, and returns the corresponding etcdBackupSpec object, and an error if there is any.
func (c *etcdBackupSpecs) Get(name string, options v1.GetOptions) (result *v1beta2.EtcdBackupSpec, err error) {
	result = &v1beta2.EtcdBackupSpec{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("etcdbackupspecs").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of EtcdBackupSpecs that match those selectors.
func (c *etcdBackupSpecs) List(opts v1.ListOptions) (result *v1beta2.EtcdBackupSpecList, err error) {
	result = &v1beta2.EtcdBackupSpecList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("etcdbackupspecs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested etcdBackupSpecs.
func (c *etcdBackupSpecs) Watch(opts v1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("etcdbackupspecs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch()
}

// Create takes the representation of a etcdBackupSpec and creates it.  Returns the server's representation of the etcdBackupSpec, and an error, if there is any.
func (c *etcdBackupSpecs) Create(etcdBackupSpec *v1beta2.EtcdBackupSpec) (result *v1beta2.EtcdBackupSpec, err error) {
	result = &v1beta2.EtcdBackupSpec{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("etcdbackupspecs").
		Body(etcdBackupSpec).
		Do().
		Into(result)
	return
}

// Update takes the representation of a etcdBackupSpec and updates it. Returns the server's representation of the etcdBackupSpec, and an error, if there is any.
func (c *etcdBackupSpecs) Update(etcdBackupSpec *v1beta2.EtcdBackupSpec) (result *v1beta2.EtcdBackupSpec, err error) {
	result = &v1beta2.EtcdBackupSpec{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("etcdbackupspecs").
		Name(etcdBackupSpec.Name).
		Body(etcdBackupSpec).
		Do().
		Into(result)
	return
}

// Delete takes name of the etcdBackupSpec and deletes it. Returns an error if one occurs.
func (c *etcdBackupSpecs) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("etcdbackupspecs").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *etcdBackupSpecs) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("etcdbackupspecs").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched etcdBackupSpec.
func (c *etcdBackupSpecs) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta2.EtcdBackupSpec, err error) {
	result = &v1beta2.EtcdBackupSpec{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("etcdbackupspecs").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
