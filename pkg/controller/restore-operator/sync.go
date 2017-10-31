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
	"fmt"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
		return nil
	}
	return r.handleCR(obj.(*api.EtcdRestore), key)
}

// handleCR takes in EtcdRestore CR and prepares the seed so that etcd operator can take over it later.
func (r *Restore) handleCR(er *api.EtcdRestore, key string) error {
	// don't process the CR if it has a status since
	// having a status means that the restore is either made or failed.
	if er.Status.Succeeded || len(er.Status.Reason) != 0 {
		return nil
	}
	err := r.prepareSeed(er)
	r.reportStatus(err, er)
	return err
}

func (r *Restore) reportStatus(rerr error, er *api.EtcdRestore) {
	if rerr != nil {
		er.Status.Succeeded = false
		er.Status.Reason = rerr.Error()
	} else {
		er.Status.Succeeded = true
	}
	_, err := r.etcdCRCli.EtcdV1beta2().EtcdRestores(r.namespace).Update(er)
	if err != nil {
		r.logger.Warningf("failed to update status of restore CR %v : (%v)", er.Name, err)
	}
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

// prepareSeed creates:
// - create EtcdCluster CR but spec.paused=true and status.phase="Running"
//  - spec.paused=true: keep operator from touching membership
// 	- status.phase=Running:
//  	1. expect operator to setup the services
//  	2. make operator ignore the "create seed member" phase
// - create seed member that would restore data from backup
// 	- ownerRef to above EtcdCluster CR
// - update EtcdCluster CR spec.paused=false
// 	- etcd operator should pick up the membership and scale the etcd cluster
func (r *Restore) prepareSeed(er *api.EtcdRestore) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("prepare seed failed: %v", err)
		}
	}()

	cs := er.Spec.ClusterSpec
	// Use the restore CR's name as the name of the etcd cluster being restored
	clusterName := er.Name

	ec := &api.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
		Spec:       cs,
	}

	ec.Spec.Paused = true
	ec.Status.Phase = api.ClusterPhaseRunning
	ec, err = r.etcdCRCli.EtcdV1beta2().EtcdClusters(r.namespace).Create(ec)
	if err != nil {
		return err
	}

	r.createSeedMember(cs, r.mySvcAddr, clusterName, ec.AsOwner())

	// Retry updating the etcdcluster CR spec.paused=false. The etcd-operator will update the CR once so there needs to be a single retry in case of conflict
	err = retryutil.Retry(2, 1, func() (bool, error) {
		ec, err = r.etcdCRCli.EtcdV1beta2().EtcdClusters(r.namespace).Get(clusterName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		ec.Spec.Paused = false
		_, err = r.etcdCRCli.EtcdV1beta2().EtcdClusters(r.namespace).Update(ec)
		if err != nil {
			if apierrors.IsConflict(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to update etcdcluster CR to spec.paused=false: %v", err)
	}
	return nil
}

func (r *Restore) createSeedMember(cs api.ClusterSpec, svcAddr, clusterName string, owner metav1.OwnerReference) error {
	m := &etcdutil.Member{
		Name:      etcdutil.CreateMemberName(clusterName, 0),
		Namespace: r.namespace,
		// TODO: support TLS
		SecurePeer:   false,
		SecureClient: false,
	}
	ms := etcdutil.NewMemberSet(m)
	backupURL := backupapi.BackupURLForRestore("http", svcAddr, clusterName)
	cs.Cleanup()
	pod := k8sutil.NewSeedMemberPod(clusterName, ms, m, cs, owner, backupURL)
	_, err := r.kubecli.Core().Pods(r.namespace).Create(pod)
	return err
}
