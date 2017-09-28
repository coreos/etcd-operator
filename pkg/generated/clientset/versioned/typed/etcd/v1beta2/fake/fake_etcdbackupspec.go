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
package fake

import (
	v1beta2 "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeEtcdBackupSpecs implements EtcdBackupSpecInterface
type FakeEtcdBackupSpecs struct {
	Fake *FakeEtcdV1beta2
	ns   string
}

var etcdbackupspecsResource = schema.GroupVersionResource{Group: "etcd.database.coreos.com", Version: "v1beta2", Resource: "etcdbackupspecs"}

var etcdbackupspecsKind = schema.GroupVersionKind{Group: "etcd.database.coreos.com", Version: "v1beta2", Kind: "EtcdBackupSpec"}

// Get takes name of the etcdBackupSpec, and returns the corresponding etcdBackupSpec object, and an error if there is any.
func (c *FakeEtcdBackupSpecs) Get(name string, options v1.GetOptions) (result *v1beta2.EtcdBackupSpec, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(etcdbackupspecsResource, c.ns, name), &v1beta2.EtcdBackupSpec{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.EtcdBackupSpec), err
}

// List takes label and field selectors, and returns the list of EtcdBackupSpecs that match those selectors.
func (c *FakeEtcdBackupSpecs) List(opts v1.ListOptions) (result *v1beta2.EtcdBackupSpecList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(etcdbackupspecsResource, etcdbackupspecsKind, c.ns, opts), &v1beta2.EtcdBackupSpecList{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.EtcdBackupSpecList), err
}

// Watch returns a watch.Interface that watches the requested etcdBackupSpecs.
func (c *FakeEtcdBackupSpecs) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(etcdbackupspecsResource, c.ns, opts))

}

// Create takes the representation of a etcdBackupSpec and creates it.  Returns the server's representation of the etcdBackupSpec, and an error, if there is any.
func (c *FakeEtcdBackupSpecs) Create(etcdBackupSpec *v1beta2.EtcdBackupSpec) (result *v1beta2.EtcdBackupSpec, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(etcdbackupspecsResource, c.ns, etcdBackupSpec), &v1beta2.EtcdBackupSpec{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.EtcdBackupSpec), err
}

// Update takes the representation of a etcdBackupSpec and updates it. Returns the server's representation of the etcdBackupSpec, and an error, if there is any.
func (c *FakeEtcdBackupSpecs) Update(etcdBackupSpec *v1beta2.EtcdBackupSpec) (result *v1beta2.EtcdBackupSpec, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(etcdbackupspecsResource, c.ns, etcdBackupSpec), &v1beta2.EtcdBackupSpec{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.EtcdBackupSpec), err
}

// Delete takes name of the etcdBackupSpec and deletes it. Returns an error if one occurs.
func (c *FakeEtcdBackupSpecs) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(etcdbackupspecsResource, c.ns, name), &v1beta2.EtcdBackupSpec{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeEtcdBackupSpecs) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(etcdbackupspecsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1beta2.EtcdBackupSpecList{})
	return err
}

// Patch applies the patch and returns the patched etcdBackupSpec.
func (c *FakeEtcdBackupSpecs) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta2.EtcdBackupSpec, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(etcdbackupspecsResource, c.ns, name, data, subresources...), &v1beta2.EtcdBackupSpec{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1beta2.EtcdBackupSpec), err
}
