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
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd/clientv3"
)

var validPeerURL = regexp.MustCompile(`^\w+:\/\/[\w\.\-]+(:\d+)?$`)

func ListMembers(endpoints []string) (*clientv3.MemberListResponse, error) {
	cfg := clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberList(ctx)
	cancel()
	etcdcli.Close()
	return resp, err
}

func RemoveMember(clientURLs []string, id uint64) error {
	cfg := clientv3.Config{
		Endpoints:   clientURLs,
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	_, err = etcdcli.Cluster.MemberRemove(ctx, id)
	cancel()
	return err
}

func MemberNameFromPeerURL(pu string) (string, error) {
	// url.Parse has very loose validation. We do our own validation.
	if !validPeerURL.MatchString(pu) {
		return "", errors.New("invalid PeerURL format")
	}
	u, err := url.Parse(pu)
	if err != nil {
		return "", err
	}
	return strings.Split(u.Host, ":")[0], nil
}
