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

import api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

const (
	// Copy from deployment_controller.go:
	// maxRetries is the number of times a restore request will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// an restore request is going to be requeued:
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
)

func (r *Restore) runWorker() {
	for r.processNextItem() {
	}
}

func (r *Restore) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := r.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer r.queue.Done(key)
	err := r.processItem(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	r.handleErr(err, key)
	return true
}

func (r *Restore) processItem(key string) error {
	obj, exists, err := r.indexer.GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		cn, ok := r.clusterNames.Load(key)
		if ok {
			r.restoreCRs.Delete(cn)
			r.clusterNames.Delete(key)
		}
		return nil
	}

	er := obj.(*api.EtcdRestore)
	r.clusterNames.Store(key, er.Spec.BackupSpec.ClusterName)
	r.restoreCRs.Store(er.Spec.BackupSpec.ClusterName, er)
	// TODO: create seed member.
	return nil
}

func (r *Restore) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		r.queue.Forget(key)
		return
	}

	// This controller retries maxRetries times if something goes wrong. After that, it stops trying.
	if r.queue.NumRequeues(key) < maxRetries {
		r.logger.Errorf("error syncing restore request (%v): %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		r.queue.AddRateLimited(key)
		return
	}

	r.queue.Forget(key)
	// Report that, even after several retries, we could not successfully process this key
	r.logger.Infof("dropping restore request (%v) out of the queue: %v", key, err)
}
