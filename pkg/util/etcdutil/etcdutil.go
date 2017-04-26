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
	"crypto/tls"
	"fmt"

	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd/clientv3"

	"golang.org/x/net/context"
)

func NewClient(clientURLs []string, tc *tls.Config) (*clientv3.Client, error) {
	cfg := clientv3.Config{
		Endpoints:   clientURLs,
		DialTimeout: constants.DefaultDialTimeout,
		TLS:         tc,
	}
	return clientv3.New(cfg)
}

func ListMembers(clientURLs []string, tc *tls.Config) (*clientv3.MemberListResponse, error) {
	etcdcli, err := NewClient(clientURLs, tc)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberList(ctx)
	cancel()
	etcdcli.Close()
	return resp, err
}

func RemoveMember(clientURLs []string, tc *tls.Config, id uint64) error {
	etcdcli, err := NewClient(clientURLs, tc)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	_, err = etcdcli.Cluster.MemberRemove(ctx, id)
	cancel()
	return err
}

func CheckHealth(url string, tc *tls.Config) (bool, error) {
	etcdcli, err := NewClient([]string{url}, tc)
	if err != nil {
		return false, fmt.Errorf("failed to create etcd client for %s: %v", url, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	_, err = etcdcli.Get(ctx, "/", clientv3.WithSerializable())
	cancel()
	etcdcli.Close()
	if err != nil {
		return false, fmt.Errorf("etcd health probing failed for %s: %v", url, err)
	}
	return true, nil
}
