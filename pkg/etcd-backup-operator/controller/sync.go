package controller

import (
	"fmt"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
)

const (
	// Copy from deployment_controller.go:
	// maxRetries is the number of times a Vault will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// a Vault is going to be requeued:
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
	// Remove fmt.Println(eb) when implementing handle backup logic.
	b.handleBackup(&eb.Spec)
	return nil
}

func (b *Backup) handleBackup(spec *api.EtcdBackupSpec) error {
	switch spec.StorageType {
	case api.BackupStorageTypeS3:
		return b.handleS3(spec.ClusterName, spec.S3)
	default:
		return fmt.Errorf("unknown StorageType: %v", spec.StorageType)
	}
}

func (b *Backup) handleS3(clusterName string, s3 *api.S3Source) error {
	s3b := &s3Backup{
		namespace:   b.namespace,
		clusterName: clusterName,
		s3bucket:    s3.S3Bucket,
		prefix:      backupapi.ToS3Prefix(s3.Prefix, b.namespace, clusterName),
		awsSecret:   s3.AWSSecret,
		kubecli:     b.kubecli,
	}
	return s3b.saveSnap()
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
		b.logger.Errorf("error syncing Vault (%v): %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		b.queue.AddRateLimited(key)
		return
	}

	b.queue.Forget(key)
	// Report that, even after several retries, we could not successfully process this key
	b.logger.Infof("Dropping Backup (%v) out of the queue: %v", key, err)
}
