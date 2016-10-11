package cluster

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/kube-etcd-controller/pkg/util/etcdutil"
	"github.com/coreos/kube-etcd-controller/pkg/util/k8sutil"
)

func (c *Cluster) upgradeOneMember(m *etcdutil.Member) error {
	logrus.Infof("upgrading an member: %v", m.Name)
	pod, err := c.kclient.Pods(c.namespace).Get(m.Name)
	if err != nil {
		return fmt.Errorf("fail to get pod (%s): %v", m.Name, err)
	}
	pod.Spec.Containers[0].Image = k8sutil.MakeEtcdImage(c.spec.Version)
	k8sutil.SetEtcdVersion(pod, c.spec.Version)
	_, err = c.kclient.Pods(c.namespace).Update(pod)
	if err != nil {
		return fmt.Errorf("fail to update pod (%s): %v", m.Name, err)
	}
	return nil
}
