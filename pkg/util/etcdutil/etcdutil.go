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
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd/clientv3"

	"golang.org/x/net/context"
)

var validPeerURL = regexp.MustCompile(`^\w+:\/\/[\w\.\-]+(:\d+)?$`)

func ListMembers(tlsCfg *tls.Config, endpoints []string) (*clientv3.MemberListResponse, error) {
	etcdcli, err := EtcdClient(tlsCfg, endpoints)
	if err != nil {
		return nil, err
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberList(ctx)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("error listing members: %v", err)
	}
	return resp, nil
}

func RemoveMember(tlsCfg *tls.Config, clientURLs []string, id uint64) error {
	etcdcli, err := EtcdClient(tlsCfg, clientURLs)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	_, err = etcdcli.Cluster.MemberRemove(ctx, id)
	cancel()
	if err != nil {
		return fmt.Errorf("error removing member: %v", err)
	}
	return err
}

func EtcdClient(tlsCfg *tls.Config, endpoints []string) (*clientv3.Client, error) {
	cfg := clientv3.Config{
		Endpoints:   endpoints,
		TLS:         tlsCfg,
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating etcd client: %v", err)
	}
	return etcdcli, nil
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

func CheckHealth(url string) (bool, error) {
	cfg := clientv3.Config{
		Endpoints:   []string{url},
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return false, fmt.Errorf("failed to create etcd client for %s: %v", url, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	_, err = etcdcli.Get(ctx, "/", clientv3.WithSerializable())
	cancel()
	etcdcli.Close()
	if err != nil {
		return false, fmt.Errorf("etcd health probing failed for %s: %v ", url, err)
	}
	return true, nil
}
