package controller

import (
	"context"
	"os"

	"github.com/coreos/etcd-operator/pkg/client"
	"github.com/coreos/etcd-operator/pkg/generated/clientset/versioned"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Backup struct {
	logger *logrus.Entry

	namespace string
	name      string
	// k8s workqueue pattern
	indexer  cache.Indexer
	informer cache.Controller
	queue    workqueue.RateLimitingInterface

	kubecli       kubernetes.Interface
	backupCRCli   versioned.Interface
	kubeExtClient apiextensionsclient.Interface
}

// New creates a backup operator.
func New() *Backup {
	return &Backup{
		logger:        logrus.WithField("pkg", "controller"),
		namespace:     os.Getenv(constants.EnvOperatorPodNamespace),
		name:          os.Getenv(constants.EnvOperatorPodName),
		kubecli:       k8sutil.MustNewKubeClient(),
		backupCRCli:   client.MustNewInCluster(),
		kubeExtClient: k8sutil.MustNewKubeExtClient(),
	}
}

// Start starts the Backup operator.
func (b *Backup) Start(ctx context.Context) error {
	// TODO: check if crd exists.
	go b.run(ctx)
	<-ctx.Done()
	return ctx.Err()
}
