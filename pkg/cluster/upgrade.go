// Copyright 2016 The etcd-operator Authors
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

package cluster

import (
	"fmt"

	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
)

func (c *Cluster) upgradeOneMember(m *etcdutil.Member) error {
	pod, err := c.kclient.Pods(c.namespace).Get(m.Name)
	if err != nil {
		return fmt.Errorf("fail to get pod (%s): %v", m.Name, err)
	}
	logrus.Infof("upgrading the etcd member %v from %s to %s", m.Name, k8sutil.GetEtcdVersion(pod), c.spec.Version)
	pod.Spec.Containers[0].Image = k8sutil.MakeEtcdImage(c.spec.Version)
	k8sutil.SetEtcdVersion(pod, c.spec.Version)
	_, err = c.kclient.Pods(c.namespace).Update(pod)
	if err != nil {
		return fmt.Errorf("fail to update the etcd member (%s): %v", m.Name, err)
	}
	logrus.Infof("finished upgrading the etcd member %v", m.Name)
	return nil
}
