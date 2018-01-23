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
	"fmt"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup"

	"k8s.io/apimachinery/pkg/fields"
	kwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

// Event define backup Event
type Event struct {
	Type   kwatch.EventType
	Key    string
	Object *api.EtcdBackup
}

func (c *Controller) run(ctx context.Context) {
	source := cache.NewListWatchFromClient(
		c.backupCRCli.EtcdV1beta2().RESTClient(),
		api.EtcdBackupResourcePlural,
		c.namespace,
		fields.Everything(),
	)

	_, informer := cache.NewIndexerInformer(source, &api.EtcdBackup{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAddEtcdBackup,
		UpdateFunc: c.onUpdateEtcdBackup,
		DeleteFunc: c.onDeleteEtcdBackup,
	}, cache.Indexers{})

	c.logger.Info("starting backup controller")
	informer.Run(ctx.Done())
}

func (c *Controller) onAddEtcdBackup(obj interface{}) {
	event := &Event{
		Type:   kwatch.Added,
		Object: obj.(*api.EtcdBackup),
	}
	err := c.handleBackupEvent(event)
	if err != nil {
		c.logger.Errorf("Failed to process Add event for EtcdBackup (%s)", event.Object.Name)
	}
}

func (c *Controller) onUpdateEtcdBackup(oldObj interface{}, newObj interface{}) {
	c.logger.Warning("Update on existing etcd backup instance is not supported")
}

func (c *Controller) onDeleteEtcdBackup(obj interface{}) {
	event := &Event{
		Type:   kwatch.Deleted,
		Object: obj.(*api.EtcdBackup),
	}
	err := c.handleBackupEvent(event)
	if err != nil {
		c.logger.Errorf("Failed to process Delete event for EtcdBackup (%s)", event.Object.Name)
	}
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
