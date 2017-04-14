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

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"

	"k8s.io/client-go/pkg/api/v1"
)

func (c *Cluster) updateMembers(known etcdutil.MemberSet) error {
	resp, err := etcdutil.ListMembers(known.ClientURLs())
	if err != nil {
		return err
	}
	members := etcdutil.MemberSet{}
	for _, m := range resp.Members {
		var name string
		if c.cluster.Spec.SelfHosted != nil {
			name = m.Name
			if len(name) == 0 || len(m.ClientURLs) == 0 {
				c.logger.Errorf("member peerURL (%s): %v", m.PeerURLs[0], errUnexpectedUnreadyMember)
				return errUnexpectedUnreadyMember
			}

			curl := m.ClientURLs[0]
			bcurl := c.cluster.Spec.SelfHosted.BootMemberClientEndpoint
			if curl == bcurl {
				return fmt.Errorf("skipping update members for self hosted cluster: waiting for the boot member (%s) to be removed...", m.Name)
			}
		} else {
			name, err = etcdutil.MemberNameFromPeerURL(m.PeerURLs[0])
			if err != nil {
				c.logger.Errorf("invalid member peerURL (%s): %v", m.PeerURLs[0], err)
				return errInvalidMemberName
			}
		}
		ct, err := etcdutil.GetCounterFromMemberName(name)
		if err != nil {
			c.logger.Errorf("invalid member name (%s): %v", name, err)
			return errInvalidMemberName
		}
		if ct+1 > c.memberCounter {
			c.memberCounter = ct + 1
		}

		members[name] = &etcdutil.Member{
			Name:       name,
			Namespace:  c.cluster.Metadata.Namespace,
			ID:         m.ID,
			ClientURLs: m.ClientURLs,
			PeerURLs:   m.PeerURLs,
			SecurePeer: c.isSecurePeer(),
		}
	}
	c.members = members
	return nil
}

func podsToMemberSet(pods []*v1.Pod, selfHosted *spec.SelfHostedPolicy) etcdutil.MemberSet {
	members := etcdutil.MemberSet{}
	for _, pod := range pods {
		m := &etcdutil.Member{Name: pod.Name, Namespace: pod.Namespace}
		if selfHosted != nil {
			m.ClientURLs = []string{"http://" + pod.Status.PodIP + ":2379"}
			m.PeerURLs = []string{"http://" + pod.Status.PodIP + ":2380"}
		}
		members.Add(m)
	}
	return members
}
