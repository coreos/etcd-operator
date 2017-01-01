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

package etcdutil

import (
	"fmt"
	"strconv"
	"strings"
)

type Member struct {
	Name string
	// ID field can be 0, which is unknown ID.
	// We know the ID of a member when we get the member information from etcd,
	// but not from Kubernetes pod list.
	ID uint64

	// PeerURLs is only used for self-hosted setup.
	PeerURLs []string
	// ClientURLs is only used for self-hosted setup.
	ClientURLs []string
}

func (m *Member) ClientAddr() string {
	if len(m.ClientURLs) != 0 {
		return strings.Join(m.ClientURLs, ",")
	}

	return fmt.Sprintf("http://%s:2379", m.Name)
}

func (m *Member) PeerAddr() string {
	if len(m.PeerURLs) != 0 {
		return strings.Join(m.PeerURLs, ",")
	}

	return fmt.Sprintf("http://%s:2380", m.Name)
}

type MemberSet map[string]*Member

func NewMemberSet(ms ...*Member) MemberSet {
	res := MemberSet{}
	for _, m := range ms {
		res[m.Name] = m
	}
	return res
}

// the set of all members of s1 that are not members of s2
func (ms MemberSet) Diff(other MemberSet) MemberSet {
	diff := MemberSet{}
	for n, m := range ms {
		if _, ok := other[n]; !ok {
			diff[n] = m
		}
	}
	return diff
}

// IsEqual tells whether two member sets are equal by checking
// - they have the same set of members and member equality are judged by Name only.
func (ms MemberSet) IsEqual(other MemberSet) bool {
	if ms.Size() != other.Size() {
		return false
	}
	for n := range ms {
		if _, ok := other[n]; !ok {
			return false
		}
	}
	return true
}

func (ms MemberSet) Size() int {
	return len(ms)
}

func (ms MemberSet) String() string {
	var mstring []string

	for m := range ms {
		mstring = append(mstring, m)
	}
	return strings.Join(mstring, ",")
}

func (ms MemberSet) PickOne() *Member {
	for _, m := range ms {
		return m
	}
	panic("empty")
}

func (ms MemberSet) PeerURLPairs() []string {
	ps := make([]string, 0)
	for _, m := range ms {
		ps = append(ps, fmt.Sprintf("%s=%s", m.Name, m.PeerAddr()))
	}
	return ps
}

func (ms MemberSet) Add(m *Member) {
	ms[m.Name] = m
}

func (ms MemberSet) Remove(name string) {
	delete(ms, name)
}

func (ms MemberSet) ClientURLs() []string {
	endpoints := make([]string, 0, len(ms))
	for _, m := range ms {
		endpoints = append(endpoints, m.ClientAddr())
	}
	return endpoints
}

func GetCounterFromMemberName(name string) (int, error) {
	i := strings.LastIndex(name, "-")
	if i == -1 || i+1 >= len(name) {
		return 0, fmt.Errorf("name (%s) does not contain '-' or anything after", name)
	}
	c, err := strconv.Atoi(name[i+1:])
	if err != nil {
		return 0, err
	}
	return c, nil
}

func CreateMemberName(clusterName string, member int) string {
	return fmt.Sprintf("%s-%04d", clusterName, member)
}
