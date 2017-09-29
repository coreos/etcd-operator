package controller

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

	"k8s.io/client-go/kubernetes/scheme"
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

	cobj, err := scheme.Scheme.DeepCopy(obj)
	if err != nil {
		return err
	}
	eb := cobj.(*api.EtcdBackup)
	b.handleBackup(&eb.Spec)
	return nil
}

func (b *Backup) handleBackup(spec *api.EtcdBackupSpec) error {
	switch spec.StorageType {
	case "s3":
		return b.handleS3(spec.ClusterName, spec.S3)
	default:
		return fmt.Errorf("unknown StorageType %v", spec.StorageType)
	}
}

// don't retry on any backup failure.
const maxRetries = 0

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
		logrus.Errorf("error syncing Vault (%v): %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		b.queue.AddRateLimited(key)
		return
	}

	b.queue.Forget(key)
	// Report that, even after several retries, we could not successfully process this key
	logrus.Infof("Dropping Backup (%v) out of the queue: %v", key, err)
}

// func (b *Backup) backupStatus() {
// 	// TODO
// }
