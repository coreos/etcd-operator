package controller

import (
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
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

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}

	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)
	err := c.processItem(key.(string))
	c.handleErr(err, key)
	return true
}

func (c *Controller) processItem(key string) error {
	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		return err
	}

	if !exists {
		if _, ok := c.clusters[key]; !ok {
			return nil
		}
		c.clusters[key].Delete()
		delete(c.clusters, key)
		clustersDeleted.Inc()
		clustersTotal.Dec()
		return nil
	}

	clus := obj.(*api.EtcdCluster)

	ev := &Event{
		Type:   kwatch.Added,
		Object: clus,
	}

	if clus.DeletionTimestamp != nil {
		ev.Type = kwatch.Deleted

		pt.start()
		_, err := c.handleClusterEvent(ev)
		if err != nil {
			c.logger.Warningf("fail to handle event: %v", err)
		}
		pt.stop()
	}

	// re-watch or restart could give ADD event.
	// If for an ADD event the cluster spec is invalid then it is not added to the local cache
	// so modifying that cluster will result in another ADD event
	if _, ok := c.clusters[getNamespacedName(clus)]; ok {
		ev.Type = kwatch.Modified
	}

	pt.start()
	_, err = c.handleClusterEvent(ev)
	if err != nil {
		c.logger.Warningf("fail to handle event: %v", err)
		return err
	}
	pt.stop()
	return nil
}

func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries maxRetries times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < maxRetries {
		c.logger.Errorf("error syncing etcd cluster (%v): %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	// Report that, even after several retries, we could not successfully process this key
	c.logger.Infof("Dropping etcd cluster(%v) out of the queue: %v", key, err)
}
