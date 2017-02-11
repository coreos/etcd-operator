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

package garbagecollection

import (
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/Sirupsen/logrus"
	"k8s.io/client-go/1.5/kubernetes"
)

// RemoveDeletedMemberServices removes given cluster's member services whose corresponding members
// not belonging to given membership.
// These garbage services could have been left due to failure cases like:
// - created a service but failed to create the pod
// - failed to remove the service before
func RemoveDeletedMemberServices(clusterName, ns string, kubecli kubernetes.Interface, members etcdutil.MemberSet, logger *logrus.Entry) {
	opt := k8sutil.ClusterListOpt(clusterName)
	svcList, err := kubecli.Core().Services(ns).List(opt)
	if err != nil {
		logger.Errorf("RemoveDeletedMemberServices: fail to list services: %v", err)
		return
	}
	for i := range svcList.Items {
		svc := svcList.Items[i]
		name, ok := svc.Labels["etcd_node"]
		if !ok {
			continue // not member service
		}
		if members.Has(name) {
			continue
		}
		err := kubecli.Core().Services(ns).Delete(name, nil)
		if err != nil {
			logger.Errorf("RemoveDeletedMemberServices: fail to delete service (%s): %v", name, err)
			continue
		}
		logger.Infof("RemoveDeletedMemberServices: deleted service for deleted member (%s)", name)
	}
}
