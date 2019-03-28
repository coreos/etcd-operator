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
	"errors"
	"reflect"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/constants"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

	if eb.DeletionTimestamp != nil {
		b.deletePeriodicBackupRunner(eb.ObjectMeta.UID)
		return b.removeFinalizerOfPeriodicBackup(eb)
	}
	isPeriodic := isPeriodicBackup(&eb.Spec)

	// don't process the CR if it has a status since
	// having a status means that the backup is either made or failed.
	if !isPeriodic &&
		(eb.Status.Succeeded || len(eb.Status.Reason) != 0) {
		return nil
	}

	if isPeriodic && b.isChanged(eb) {
		// Stop previous backup runner if it exists
		b.deletePeriodicBackupRunner(eb.ObjectMeta.UID)

		// Add finalizer if need
		eb, err = b.addFinalizerOfPeriodicBackupIfNeed(eb)
		if err != nil {
			return err
		}

		// Run new backup runner
		ticker := time.NewTicker(
			time.Duration(eb.Spec.BackupPolicy.BackupIntervalInSecond) * time.Second)
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		go b.periodicRunnerFunc(ctx, ticker, eb)

		// Store cancel function for periodic
		b.backupRunnerStore.Store(eb.ObjectMeta.UID, BackupRunner{eb.Spec, cancel})

	} else if !isPeriodic {
		// Perform backup
		bs, err := b.handleBackup(nil, &eb.Spec, false)
		// Report backup status
		b.reportBackupStatus(bs, err, eb)
	}
	return err
}

func (b *Backup) isChanged(eb *api.EtcdBackup) bool {
	backupRunner, exists := b.backupRunnerStore.Load(eb.ObjectMeta.UID)
	if !exists {
		return true
	}
	return !reflect.DeepEqual(eb.Spec, backupRunner.(BackupRunner).spec)
}

func (b *Backup) deletePeriodicBackupRunner(uid types.UID) bool {
	backupRunner, exists := b.backupRunnerStore.Load(uid)
	if exists {
		backupRunner.(BackupRunner).cancelFunc()
		b.backupRunnerStore.Delete(uid)
		return true
	}
	return false
}

func (b *Backup) addFinalizerOfPeriodicBackupIfNeed(eb *api.EtcdBackup) (*api.EtcdBackup, error) {
	ebNew := eb.DeepCopyObject()
	metadata, err := meta.Accessor(ebNew)
	if err != nil {
		return eb, err
	}
	if !containsString(metadata.GetFinalizers(), "backup-operator-periodic") {
		metadata.SetFinalizers(append(metadata.GetFinalizers(), "backup-operator-periodic"))
		_, err := b.backupCRCli.EtcdV1beta2().EtcdBackups(b.namespace).Update(ebNew.(*api.EtcdBackup))
		if err != nil {
			return eb, err
		}
		return ebNew.(*api.EtcdBackup), nil
	}
	return eb, nil
}

func (b *Backup) removeFinalizerOfPeriodicBackup(eb *api.EtcdBackup) error {
	ebNew := eb.DeepCopyObject()
	metadata, err := meta.Accessor(ebNew)
	if err != nil {
		return err
	}
	var finalizers []string
	for _, finalizer := range metadata.GetFinalizers() {
		if finalizer == "backup-operator-periodic" {
			continue
		}
		finalizers = append(finalizers, finalizer)
	}
	metadata.SetFinalizers(finalizers)
	_, err = b.backupCRCli.EtcdV1beta2().EtcdBackups(b.namespace).Update(ebNew.(*api.EtcdBackup))
	return err
}

func (b *Backup) periodicRunnerFunc(ctx context.Context, t *time.Ticker, eb *api.EtcdBackup) {
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			break
		case <-t.C:
			var latestEb *api.EtcdBackup
			var bs *api.BackupStatus
			var err error
			retryLimit := 5
			for i := 1; i < retryLimit+1; i++ {
				latestEb, err = b.backupCRCli.EtcdV1beta2().EtcdBackups(b.namespace).Get(eb.Name, metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						b.logger.Infof("Could not find EtcdBackup. Stopping periodic backup for EtcdBackup CR %v",
							eb.Name)
						break
					}
					b.logger.Warningf("[Attempt: %d/%d] Failed to get latest EtcdBackup %v : (%v)",
						i, retryLimit, eb.Name, err)
					time.Sleep(1)
					continue
				}
				break
			}
			if err == nil {
				// Perform backup
				bs, err = b.handleBackup(&ctx, &latestEb.Spec, true)
			}
			// Report backup status
			b.reportBackupStatus(bs, err, latestEb)
		}
	}
}

func (b *Backup) reportBackupStatus(bs *api.BackupStatus, berr error, eb *api.EtcdBackup) {
	if berr != nil {
		eb.Status.Succeeded = false
		eb.Status.Reason = berr.Error()
	} else {
		eb.Status.Reason = ""
		eb.Status.Succeeded = true
		eb.Status.EtcdRevision = bs.EtcdRevision
		eb.Status.EtcdVersion = bs.EtcdVersion
		eb.Status.LastSuccessDate = bs.LastSuccessDate
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

func (b *Backup) handleBackup(parentContext *context.Context, spec *api.BackupSpec, isPeriodic bool) (*api.BackupStatus, error) {
	err := validate(spec)
	if err != nil {
		return nil, err
	}

	// When BackupPolicy.TimeoutInSecond <= 0, use default DefaultBackupTimeout.
	backupTimeout := time.Duration(constants.DefaultBackupTimeout)
	if spec.BackupPolicy != nil && spec.BackupPolicy.TimeoutInSecond > 0 {
		backupTimeout = time.Duration(spec.BackupPolicy.TimeoutInSecond) * time.Second
	}
	backupMaxCount := 0
	if spec.BackupPolicy != nil && spec.BackupPolicy.MaxBackups > 0 {
		backupMaxCount = spec.BackupPolicy.MaxBackups
	}

	if parentContext == nil {
		tmpParent := context.Background()
		parentContext = &tmpParent
	}
	ctx, cancel := context.WithTimeout(*parentContext, backupTimeout)
	defer cancel()
	switch spec.StorageType {
	case api.BackupStorageTypeS3:
		bs, err := handleS3(ctx, b.kubecli, spec.S3, spec.EtcdEndpoints, spec.ClientTLSSecret,
			b.namespace, isPeriodic, backupMaxCount)
		if err != nil {
			return nil, err
		}
		return bs, nil
	case api.BackupStorageTypeABS:
		bs, err := handleABS(ctx, b.kubecli, spec.ABS, spec.EtcdEndpoints, spec.ClientTLSSecret,
			b.namespace, isPeriodic, backupMaxCount)
		if err != nil {
			return nil, err
		}
		return bs, nil
	case api.BackupStorageTypeGCS:
		bs, err := handleGCS(ctx, b.kubecli, spec.GCS, spec.EtcdEndpoints, spec.ClientTLSSecret,
			b.namespace, isPeriodic, backupMaxCount)
		if err != nil {
			return nil, err
		}
		return bs, nil
	case api.BackupStorageTypeOSS:
		bs, err := handleOSS(ctx, b.kubecli, spec.OSS, spec.EtcdEndpoints, spec.ClientTLSSecret,
			b.namespace, isPeriodic, backupMaxCount)
		if err != nil {
			return nil, err
		}
		return bs, nil
	default:
		logrus.Fatalf("unknown StorageType: %v", spec.StorageType)
	}
	return nil, nil
}

// TODO: move this to initializer
func validate(spec *api.BackupSpec) error {
	if len(spec.EtcdEndpoints) == 0 {
		return errors.New("spec.etcdEndpoints should not be empty")
	}
	if spec.BackupPolicy != nil {
		if spec.BackupPolicy.BackupIntervalInSecond < 0 {
			return errors.New("spec.BackupPolicy.BackupIntervalInSecond should not be lower than 0")
		}
		if spec.BackupPolicy.MaxBackups < 0 {
			return errors.New("spec.BackupPolicy.MaxBackups should not be lower than 0")
		}
	}
	return nil
}
