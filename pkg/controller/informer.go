package controller

import (
	"context"
	"fmt"
	"time"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/probe"

	"k8s.io/apimachinery/pkg/fields"
	kwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

// TODO: get rid of this once we use workqueue
var pt *panicTimer

func init() {
	pt = newPanicTimer(time.Minute, "unexpected long blocking (> 1 Minute) when handling cluster event")
}

func (c *Controller) Start() error {
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
	c.run()
	panic("unreachable")
}

func (c *Controller) run() {
	source := cache.NewListWatchFromClient(
		c.Config.EtcdCRCli.EtcdV1beta2().RESTClient(),
		api.CRDResourcePlural,
		c.Config.Namespace,
		fields.Everything())

	_, informer := cache.NewIndexerInformer(source, &api.EtcdCluster{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAddEtcdClus,
		UpdateFunc: c.onUpdateEtcdClus,
		DeleteFunc: c.onDeleteEtcdClus,
	}, cache.Indexers{})

	ctx := context.TODO()
	// TODO: use workqueue to avoid blocking
	informer.Run(ctx.Done())
}

func (c *Controller) initResource() error {
	err := c.initCRD()
	if err != nil && !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
		return fmt.Errorf("fail to create CRD: %v", err)
	}
	if c.Config.PVProvisioner != constants.PVProvisionerNone {
		err = k8sutil.CreateStorageClass(c.KubeCli, c.PVProvisioner)
		if err != nil && !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return fmt.Errorf("fail to create storage class: %v", err)
		}
	}
	return nil
}

func (c *Controller) onAddEtcdClus(obj interface{}) {
	c.syncEtcdClus(obj.(*api.EtcdCluster))
}

func (c *Controller) onUpdateEtcdClus(oldObj, newObj interface{}) {
	c.syncEtcdClus(newObj.(*api.EtcdCluster))
}

func (c *Controller) onDeleteEtcdClus(obj interface{}) {
	clus, ok := obj.(*api.EtcdCluster)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			panic(fmt.Sprintf("unknown object from EtcdCluster delete event: %#v", obj))
		}
		clus, ok = tombstone.Obj.(*api.EtcdCluster)
		if !ok {
			panic(fmt.Sprintf("Tombstone contained object that is not an EtcdCluster: %#v", obj))
		}
	}
	ev := &Event{
		Type:   kwatch.Deleted,
		Object: clus,
	}

	pt.start()
	err := c.handleClusterEvent(ev)
	if err != nil {
		c.logger.Warningf("fail to handle event: %v", err)
	}
	pt.stop()
}

func (c *Controller) syncEtcdClus(clus *api.EtcdCluster) {
	ev := &Event{
		Type:   kwatch.Added,
		Object: clus,
	}
	if _, ok := c.clusters[clus.Name]; ok { // re-watch or restart could give ADD event
		ev.Type = kwatch.Modified
	}

	pt.start()
	err := c.handleClusterEvent(ev)
	if err != nil {
		c.logger.Warningf("fail to handle event: %v", err)
	}
	pt.stop()
}
