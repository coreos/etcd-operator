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

func (b *Backup) run(ctx context.Context) {
	source := cache.NewListWatchFromClient(
		b.backupCRCli.EtcdV1beta2().RESTClient(),
		api.EtcdBackupResourcePlural,
		b.namespace,
		fields.Everything(),
	)

	b.queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "etcd-backup-operator")
	b.indexer, b.informer = cache.NewIndexerInformer(source, &api.EtcdBackup{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc:    b.onAdd,
		UpdateFunc: b.onUpdate,
		DeleteFunc: b.onDelete,
	}, cache.Indexers{})

	defer b.queue.ShutDown()

	b.logger.Info("starting backup controller")
	go b.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), b.informer.HasSynced) {
		return
	}

	const numWorkers = 1
	for i := 0; i < numWorkers; i++ {
		go wait.Until(b.runWorker, time.Second, ctx.Done())
	}

	<-ctx.Done()
	b.logger.Info("stopping backup controller")
}

func (b *Backup) onAdd(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		panic(err)
	}
	b.queue.Add(key)
}

func (b *Backup) onUpdate(oldObj, newObj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(newObj)
	if err != nil {
		panic(err)
	}
	b.queue.Add(key)
}

func (b *Backup) onDelete(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		panic(err)
	}
	b.queue.Add(key)
}
