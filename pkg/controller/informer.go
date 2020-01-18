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
	"fmt"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/probe"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// TODO: get rid of this once we use workqueue
var pt *panicTimer

func init() {
	pt = newPanicTimer(time.Minute, "unexpected long blocking (> 1 Minute) when handling cluster event")
}

func (c *Controller) Start(ctx context.Context) error {
	// TODO: get rid of this init code. CRD and storage class will be managed outside of operator.
	for {
		err := c.initResource()
		if err == nil {
			break
		}
		c.logger.Errorf("initialization failed: %v", err)
		c.logger.Infof("retry in %v...", initRetryWaitTime)
		time.Sleep(initRetryWaitTime)
	}

	probe.SetReady()
	go c.run(ctx)
	<-ctx.Done()
	return ctx.Err()
}

func (c *Controller) run(ctx context.Context) {
	var ns string
	if c.Config.ClusterWide {
		ns = metav1.NamespaceAll
	} else {
		ns = c.Config.Namespace
	}

	source := cache.NewListWatchFromClient(
		c.Config.EtcdCRCli.EtcdV1beta2().RESTClient(),
		api.EtcdClusterResourcePlural,
		ns,
		fields.Everything())

	c.queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "etcd-operator")
	c.indexer, c.informer = cache.NewIndexerInformer(source, &api.EtcdCluster{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAddEtcdClus,
		UpdateFunc: c.onUpdateEtcdClus,
		DeleteFunc: c.onDeleteEtcdClus,
	}, cache.Indexers{})

	defer c.queue.ShutDown()

	c.logger.Info("starting etcd controller")
	go c.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		return
	}

	const numWorkers = 1
	for i := 0; i < numWorkers; i++ {
		go wait.Until(c.runWorker, time.Second, ctx.Done())
	}

	<-ctx.Done()
	c.logger.Info("stopping etcd controller")
}

func (c *Controller) initResource() error {
	if c.Config.CreateCRD {
		err := c.initCRD()
		if err != nil {
			return fmt.Errorf("fail to init CRD: %v", err)
		}
	}
	return nil
}

func (c *Controller) onAddEtcdClus(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		panic(err)
	}
	c.queue.Add(key)
}

func (c *Controller) onUpdateEtcdClus(oldObj, newObj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(newObj)
	if err != nil {
		panic(err)
	}
	c.queue.Add(key)
}

func (c *Controller) onDeleteEtcdClus(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		panic(err)
	}
	c.queue.Add(key)
}

func (c *Controller) managed(clus *api.EtcdCluster) bool {
	if v, ok := clus.Annotations[k8sutil.AnnotationScope]; ok {
		if c.Config.ClusterWide {
			return v == k8sutil.AnnotationClusterWide
		}
	} else {
		if !c.Config.ClusterWide {
			return true
		}
	}
	return false
}
