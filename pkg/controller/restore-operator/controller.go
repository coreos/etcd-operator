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

package controller

import (
	"context"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func (r *Restore) run(ctx context.Context) {
	source := cache.NewListWatchFromClient(
		r.etcdCRCli.EtcdV1beta2().RESTClient(),
		api.EtcdRestoreResourcePlural,
		r.namespace,
		fields.Everything(),
	)

	r.queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "etcd-restore-operator")
	r.indexer, r.informer = cache.NewIndexerInformer(source, &api.EtcdRestore{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc:    r.onAdd,
		UpdateFunc: r.onUpdate,
		DeleteFunc: r.onDelete,
	}, cache.Indexers{})

	defer r.queue.ShutDown()

	r.logger.Info("starting restore controller")
	go r.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), r.informer.HasSynced) {
		return
	}

	const numWorkers = 1
	for i := 0; i < numWorkers; i++ {
		go wait.Until(r.runWorker, time.Second, ctx.Done())
	}

	<-ctx.Done()
	r.logger.Info("stopping restore controller")
}

func (r *Restore) onAdd(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		panic(err)
	}
	r.queue.Add(key)
}

func (r *Restore) onUpdate(oldObj, newObj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(newObj)
	if err != nil {
		panic(err)
	}
	r.queue.Add(key)
}

func (r *Restore) onDelete(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		panic(err)
	}
	r.queue.Add(key)
}
