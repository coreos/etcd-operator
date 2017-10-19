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
	"sync"

	"github.com/coreos/etcd-operator/pkg/client"
	"github.com/coreos/etcd-operator/pkg/generated/clientset/versioned"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Restore struct {
	logger *logrus.Entry

	namespace string
	mySvcAddr string
	// k8s workqueue pattern
	indexer  cache.Indexer
	informer cache.Controller
	queue    workqueue.RateLimitingInterface

	kubecli      kubernetes.Interface
	restoreCRCli versioned.Interface

	// restoreCRs is a map of cluster name to restore cr.
	restoreCRs sync.Map
	// clusterNames is map of informer's indexer keys to cluster names
	clusterNames sync.Map
}

// New creates a restore operator.
func New(namespace, mySvcAddr string) *Restore {
	return &Restore{
		logger:       logrus.WithField("pkg", "controller"),
		namespace:    namespace,
		mySvcAddr:    mySvcAddr,
		kubecli:      k8sutil.MustNewKubeClient(),
		restoreCRCli: client.MustNewInCluster(),
	}
}

// Start starts the restore operator.
func (r *Restore) Start(ctx context.Context) error {
	go r.run(ctx)
	go r.startHTTP()
	<-ctx.Done()
	return ctx.Err()
}
