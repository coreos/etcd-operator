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
	"fmt"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup"

	kwatch "k8s.io/apimachinery/pkg/watch"
)

const (
	// Copy from deployment_controller.go:
	// maxRetries is the number of times a etcd backup will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// an etcd backup is going to be requeued:
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
)

func (b *Controller) runWorker() {
	for b.processNextItem() {
	}
}

func (b *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := b.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer b.queue.Done(key)
	err := b.processItem(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	b.handleErr(err, key)
	return true
}

func (b *Controller) processItem(key string) error {
	obj, exists, err := b.indexer.GetByKey(key)
	if err != nil {
		return err
	}
	var event *Event
	if !exists {
		event = &Event{
			Type:   kwatch.Deleted,
			Key:    key,
			Object: &api.EtcdBackup{},
		}
	} else {
		if _, ok := b.backups[key]; ok {
			b.logger.Warningf("Ignore modification on existing backup instance")
			return nil
		}
		event = &Event{
			Type:   kwatch.Added,
			Key:    key,
			Object: obj.(*api.EtcdBackup),
		}
	}

	return b.handleBackupEvent(event)
}

func (b *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		b.queue.Forget(key)
		return
	}

	// This controller retries maxRetries times if something goes wrong. After that, it stops trying.
	if b.queue.NumRequeues(key) < maxRetries {
		b.logger.Errorf("error syncing etcd backup (%v): %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		b.queue.AddRateLimited(key)
		return
	}

	b.queue.Forget(key)
	// Report that, even after several retries, we could not successfully process this key
	b.logger.Infof("Dropping etcd backup (%v) out of the queue: %v", key, err)
}

func (c *Controller) handleBackupEvent(event *Event) error {
	bkup := event.Object

	switch event.Type {
	case kwatch.Added:
		if _, ok := c.backups[event.Key]; ok {
			return fmt.Errorf("unsafe state. backup (%s) was created before but we received event (%s)", bkup.Name, event.Type)
		}
		bk := backup.New(bkup)
		c.backups[event.Key] = bk

	case kwatch.Deleted:
		if _, ok := c.backups[event.Key]; !ok {
			return fmt.Errorf("unsafe state. backup (%s) was never created but we received event (%s)", event.Key, event.Type)
		}
		c.backups[event.Key].Delete()
		delete(c.backups, event.Key)
	}
	return nil
}
