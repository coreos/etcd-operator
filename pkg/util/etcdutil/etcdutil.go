// Copyright 2016 The kube-etcd-controller Authors
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
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/coreos/kube-etcd-controller/pkg/util/constants"
	"golang.org/x/net/context"
)

func WaitMemberReady(cli *clientv3.Client, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting etcd cluster ready")
		default:
		}
		ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
		_, err := cli.Get(ctx, "/", clientv3.WithSerializable())
		if err == rpctypes.ErrNotCapable {
			time.Sleep(1 * time.Second)
			continue
		}
		return err
	}
}
