package operator

import (
	"context"
	"time"

	"github.com/coreos/etcd-operator/etcd-backup-operator/pkg/apis/backup/v1beta2"
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"

	"github.com/Sirupsen/logrus"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func (b *Backup) run(ctx context.Context) {
	source := cache.NewListWatchFromClient(
		b.backupCRCli.RESTClient(),
		v1beta2.EtcdBackupResourcePlural,
		b.namespace,
		fields.Everything(),
	)

	b.queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "backup-operator")
	b.indexer, b.informer = cache.NewIndexerInformer(source, &api.EtcdBackup{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc:    b.onAdd,
		UpdateFunc: b.onUpdate,
		DeleteFunc: b.onDelete,
	}, cache.Indexers{})

	defer b.queue.ShutDown()

	logrus.Info("starting backup controller")
	go b.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), b.informer.HasSynced) {
		logrus.Error("Timed out waiting for caches to sync")
		return
	}

	const numWorkers = 1
	for i := 0; i < numWorkers; i++ {
		go wait.Until(b.runWorker, time.Second, ctx.Done())
	}

	<-ctx.Done()
	logrus.Info("stopping backup controller")
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
	if err == nil {
		b.queue.Add(key)
	}
}
