/*
Copyright 2018 The etcd-operator Authors

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

// This file was automatically generated by informer-gen

package v1beta2

import (
	etcd_v1beta2 "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	versioned "github.com/coreos/etcd-operator/pkg/generated/clientset/versioned"
	internalinterfaces "github.com/coreos/etcd-operator/pkg/generated/informers/externalversions/internalinterfaces"
	v1beta2 "github.com/coreos/etcd-operator/pkg/generated/listers/etcd/v1beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
	time "time"
)

// EtcdBackupInformer provides access to a shared informer and lister for
// EtcdBackups.
type EtcdBackupInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1beta2.EtcdBackupLister
}

type etcdBackupInformer struct {
	factory internalinterfaces.SharedInformerFactory
}

// NewEtcdBackupInformer constructs a new informer for EtcdBackup type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewEtcdBackupInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				return client.EtcdV1beta2().EtcdBackups(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				return client.EtcdV1beta2().EtcdBackups(namespace).Watch(options)
			},
		},
		&etcd_v1beta2.EtcdBackup{},
		resyncPeriod,
		indexers,
	)
}

func defaultEtcdBackupInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewEtcdBackupInformer(client, v1.NamespaceAll, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
}

func (f *etcdBackupInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&etcd_v1beta2.EtcdBackup{}, defaultEtcdBackupInformer)
}

func (f *etcdBackupInformer) Lister() v1beta2.EtcdBackupLister {
	return v1beta2.NewEtcdBackupLister(f.Informer().GetIndexer())
}
