package operator

import (
	"context"
	"os"

	"github.com/coreos/etcd-operator/etcd-backup-operator/pkg/client"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type Backup struct {
	namespace string
	name      string
	// k8s workqueue pattern
	indexer  cache.Indexer
	informer cache.Controller
	queue    workqueue.RateLimitingInterface

	kubecli       kubernetes.Interface
	backupCRCli   client.BackupCR
	kubeExtClient apiextensionsclient.Interface
}

// New creates a backup operator.
func New() *Backup {
	return &Backup{
		namespace:     os.Getenv(constants.EnvOperatorPodNamespace),
		name:          os.Getenv(constants.EnvOperatorPodName),
		kubecli:       k8sutil.MustNewKubeClient(),
		backupCRCli:   client.MustNewInCluster(),
		kubeExtClient: k8sutil.MustNewKubeExtClient(),
	}
}

// Start starts the Backup operator.
func (b *Backup) Start(ctx context.Context) error {
	err := b.init(ctx)
	if err != nil {
		return err
	}
	go b.run(ctx)
	<-ctx.Done()
	return ctx.Err()
}

func (b *Backup) init(ctx context.Context) error {
	// TODO: check if crd exists
	return nil
}
