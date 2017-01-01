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
	"errors"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"

	"k8s.io/kubernetes/pkg/api"
)

var errMemberNotReady = errors.New("etcd member's Name is empty. It's not ready yet.")

func (c *Cluster) updateMembers(known etcdutil.MemberSet) error {
	resp, err := etcdutil.ListMembers(known.ClientURLs())
	if err != nil {
		return err
	}
	members := etcdutil.MemberSet{}
	for _, m := range resp.Members {
		if len(m.Name) == 0 {
			c.logger.Errorf("member (%x): %v. Will retry later...", m.ID, errMemberNotReady)
			return errMemberNotReady
		}
		name := m.Name
		ct, err := etcdutil.GetCounterFromMemberName(name)
		if err != nil {
			c.logger.Errorf("fail to parse counter from name (%s): %v", name, err)
			return errInvalidMemberName
		}
		if ct+1 > c.memberCounter {
			c.memberCounter = ct + 1
		}

		members[name] = &etcdutil.Member{
			Name:       name,
			ID:         m.ID,
			ClientURLs: m.ClientURLs,
			PeerURLs:   m.PeerURLs,
		}
	}
	c.members = members
	return nil
}

func podsToMemberSet(pods []*api.Pod, selfHosted *spec.SelfHostedPolicy) etcdutil.MemberSet {
	members := etcdutil.MemberSet{}
	for _, pod := range pods {
		m := &etcdutil.Member{Name: pod.Name}
		if selfHosted != nil {
			m.ClientURLs = []string{"http://" + pod.Status.PodIP + ":2379"}
			m.PeerURLs = []string{"http://" + pod.Status.PodIP + ":2380"}
		}
		members.Add(m)
	}
	return members
}
