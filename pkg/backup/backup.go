package backup

import (
	"time"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/kube-etcd-controller/pkg/util/etcdutil"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

type Backup struct {
	kclient     *unversioned.Client
	clusterName string
	policy      Policy
}

func New(kclient *unversioned.Client, clusterName string, policy Policy) *Backup {
	return &Backup{
		kclient:     kclient,
		clusterName: clusterName,
		policy:      policy,
	}
}

func (b *Backup) Run() {
	for {
		// TODO: add interval to backup policy
		time.Sleep(20 * time.Second)

		pods, err := b.kclient.Pods("default").List(api.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"app":          "etcd",
				"etcd_cluster": b.clusterName,
			}),
		})
		if err != nil {
			panic(err)
		}
		for i := range pods.Items {
			m := etcdutil.Member{Name: pods.Items[i].Name}
			cfg := clientv3.Config{
				Endpoints:   []string{m.ClientAddr()},
				DialTimeout: 5 * time.Second,
			}
			etcdcli, err := clientv3.New(cfg)
			if err != nil {
				logrus.Errorf("clientv3.New failed: %v", err)
				continue
			}
			resp, err := etcdcli.Get(context.TODO(), "/", clientv3.WithSerializable())
			if err != nil {
				logrus.Errorf("etcdcli.Get failed: %v", err)
				continue
			}
			logrus.Infof("member: %s, revision: %d", m.Name, resp.Header.Revision)
		}
	}
}
