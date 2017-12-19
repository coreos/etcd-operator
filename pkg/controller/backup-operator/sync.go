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
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

	"github.com/sirupsen/logrus"
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

func (b *Backup) runWorker() {
	for b.processNextItem() {
	}
}

func (b *Backup) processNextItem() bool {
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

func (b *Backup) processItem(key string) error {
	obj, exists, err := b.indexer.GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	eb := obj.(*api.EtcdBackup)
	// don't process the CR if it has a status since
	// having a status means that the backup is either made or failed.
	if eb.Status.Succeeded || len(eb.Status.Reason) != 0 {
		return nil
	}
	bs, err := b.handleBackup(&eb.Spec)
	// Report backup status
	b.reportBackupStatus(bs, err, eb)
	return err
}

func (b *Backup) reportBackupStatus(bs *api.BackupStatus, berr error, eb *api.EtcdBackup) {
	if berr != nil {
		eb.Status.Succeeded = false
		eb.Status.Reason = berr.Error()
	} else {
		eb.Status.Succeeded = true
		eb.Status.EtcdRevision = bs.EtcdRevision
		eb.Status.EtcdVersion = bs.EtcdVersion
	}
	_, err := b.backupCRCli.EtcdV1beta2().EtcdBackups(b.namespace).Update(eb)
	if err != nil {
		b.logger.Warningf("failed to update status of backup CR %v : (%v)", eb.Name, err)
	}
}

func (b *Backup) handleErr(err error, key interface{}) {
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

func (b *Backup) handleBackup(spec *api.BackupSpec) (*api.BackupStatus, error) {
	switch spec.StorageType {
	case api.BackupStorageTypeS3:
		bs, err := handleS3(b.kubecli, spec.S3, spec.EtcdEndpoints, spec.ClientTLSSecret, b.namespace)
		if err != nil {
			return nil, err
		}
		return bs, nil
	default:
		logrus.Fatalf("unknown StorageType: %v", spec.StorageType)
	}
	return nil, nil
}
