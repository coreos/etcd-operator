package backupstorage

import (
	"os"
	"path"

	backupabs "github.com/coreos/etcd-operator/pkg/backup/abs"
	"github.com/coreos/etcd-operator/pkg/backup/abs/absconfig"
	"github.com/coreos/etcd-operator/pkg/spec"

	"github.com/coreos/etcd-operator/pkg/backup/env"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type abs struct {
	absconfig.ABSContext
	clusterName  string
	namespace    string
	backupPolicy spec.BackupPolicy
	kubecli      kubernetes.Interface
	abscli       *backupabs.ABS
}

// NewABSStorage returns a new ABS Storage implementation using the given absCtx, kubecli, cluster name, namespace and backup policy
func NewABSStorage(absCtx absconfig.ABSContext, kubecli kubernetes.Interface, clusterName, ns string, p spec.BackupPolicy) (Storage, error) {
	prefix := path.Join(ns, clusterName)

	abscli, err := func() (*backupabs.ABS, error) {
		if p.ABS != nil {
			err := setupABSCreds(kubecli, ns, p.ABS.ABSSecret)
			if err != nil {
				return nil, err
			}
			return backupabs.New(p.ABS.ABSContainer, prefix)
		}
		return backupabs.New(absCtx.ABSContainer, prefix)
	}()
	if err != nil {
		return nil, err
	}

	ws := &abs{
		ABSContext:   absCtx,
		kubecli:      kubecli,
		clusterName:  clusterName,
		backupPolicy: p,
		namespace:    ns,
		abscli:       abscli,
	}
	return ws, nil
}

func (a *abs) Create() error {
	// TODO: check if container exists?
	return nil
}

func (a *abs) Clone(from string) error {
	prefix := a.namespace + "/" + from
	return a.abscli.CopyPrefix(prefix)
}

func (a *abs) Delete() error {
	if a.backupPolicy.AutoDelete {
		names, err := a.abscli.List()
		if err != nil {
			return err
		}
		for _, n := range names {
			err = a.abscli.Delete(n)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func setupABSCreds(kubecli kubernetes.Interface, ns, secret string) error {
	se, err := kubecli.CoreV1().Secrets(ns).Get(secret, metav1.GetOptions{})
	if err != nil {
		return err
	}

	os.Setenv(env.ABSStorageAccount, string(se.Data[spec.ABSStorageAccount]))
	os.Setenv(env.ABSStorageKey, string(se.Data[spec.ABSStorageKey]))

	return nil
}
