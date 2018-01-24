package backup

import (
	"os"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/client"
	"github.com/coreos/etcd-operator/pkg/generated/clientset/versioned"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

type Backup struct {
	logger    *logrus.Entry
	namespace string

	backup *api.EtcdBackup

	stopCh chan struct{}

	backupCRCli versioned.Interface
	kubecli     kubernetes.Interface
}

func New(bk *api.EtcdBackup) *Backup {
	lg := logrus.WithField("pkg", "backup").WithField("backup-name", bk.Name)

	b := &Backup{
		logger:      lg,
		namespace:   os.Getenv(constants.EnvOperatorPodNamespace),
		backup:      bk,
		stopCh:      make(chan struct{}),
		kubecli:     k8sutil.MustNewKubeClient(),
		backupCRCli: client.MustNewInCluster(),
	}
	go func() {
		b.run()
	}()
	return b
}

func (bk *Backup) run() {
	bs, err := bk.handleBackup()
	bk.reportBackupStatus(bs, err)
	// TODO: backup control loop will be added here
	// It will be used for backup schedule
}

func (bk *Backup) Delete() {
	bk.logger.Info("backup is deleted by user")
	close(bk.stopCh)
}

func (bk *Backup) handleBackup() (*api.BackupStatus, error) {
	spec := bk.backup.Spec
	switch spec.StorageType {
	case api.BackupStorageTypeS3:
		bs, err := handleS3(bk.kubecli, spec.S3, spec.EtcdEndpoints, spec.ClientTLSSecret, bk.namespace)
		if err != nil {
			return nil, err
		}
		return bs, nil
	default:
		logrus.Fatalf("unknown StorageType: %v", spec.StorageType)
	}
	return nil, nil
}

func (bk *Backup) reportBackupStatus(bs *api.BackupStatus, berr error) {
	if berr != nil {
		bk.backup.Status.Succeeded = false
		bk.backup.Status.Reason = berr.Error()
	} else {
		bk.backup.Status.Succeeded = true
		bk.backup.Status.EtcdRevision = bs.EtcdRevision
		bk.backup.Status.EtcdVersion = bs.EtcdVersion
	}
	_, err := bk.backupCRCli.EtcdV1beta2().EtcdBackups(bk.namespace).Update(bk.backup)
	if err != nil {
		bk.logger.Warningf("failed to update status of backup CR %v : (%v)", bk.backup.Name, err)
	}
}
