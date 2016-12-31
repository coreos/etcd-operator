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
	"net/url"
	"strings"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"

	"github.com/coreos/etcd/etcdserver/etcdserverpb"
	"k8s.io/kubernetes/pkg/api"
)

var errMemberNotReady = errors.New("etcd member's Name is empty. It's not ready")

func (c *Cluster) updateMembers(known etcdutil.MemberSet) error {
	resp, err := etcdutil.ListMembers(known.ClientURLs())
	if err != nil {
		return err
	}
	members := etcdutil.MemberSet{}
	for _, m := range resp.Members {
		if len(m.Name) == 0 {
			c.logger.Errorf("member (%v): %v", m.PeerURLs[0], errMemberNotReady)
			return errMemberNotReady
		}
		id := findID(m.Name)
		if id+1 > c.idCounter {
			c.idCounter = id + 1
		}

		members[m.Name] = &etcdutil.Member{
			Name:       m.Name,
			ID:         m.ID,
			ClientURLs: m.ClientURLs,
			PeerURLs:   m.PeerURLs,
		}
	}
	c.members = members
	return nil
}

func (c *Cluster) handleUnreadyMember(running []*api.Pod) error {
	known := podsToMemberSet(running, c.cluster.Spec.SelfHosted)
	resp, err := etcdutil.ListMembers(known.ClientURLs())
	if err != nil {
		return err
	}
	for _, m := range resp.Members {
		if len(m.Name) != 0 {
			continue
		}
		found, err := isRunningMember(m, known, c.cluster.Spec.SelfHosted)
		if err != nil {
			return err
		}
		// If this is not found from running pods, we probably failed to
		// create pod for this member.
		if !found {
			c.logger.Infof("handle unready: removing member (%v) not being found in running pods", m.PeerURLs[0])
			return etcdutil.RemoveMember(known.ClientURLs(), m.ID)
		}
	}
	return errors.New("didn't found any not ready member again. Will retry reconcile later...")
}

func isRunningMember(m *etcdserverpb.Member, running etcdutil.MemberSet, selfHosted *spec.SelfHostedPolicy) (bool, error) {
	if selfHosted != nil {
		return false, errors.New("TODO: handle self hosted unready member")
	}
	n, err := nameFromPeerURL(m.PeerURLs[0])
	if err != nil {
		return false, err
	}
	for _, p := range running {
		if n == p.Name {
			return true, nil
		}
	}
	return false, nil
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

func nameFromPeerURL(pu string) (string, error) {
	u, err := url.Parse(pu)
	if err != nil {
		return "", err
	}
	return strings.Split(u.Host, ":")[0], nil
}
