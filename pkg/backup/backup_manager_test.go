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

package backup

import (
	"io"
	"testing"

	"github.com/coreos/etcd/clientv3"
	"golang.org/x/net/context"
)

type fakeMaintenanceClient struct {
	clientv3.Maintenance
}

func (c *fakeMaintenanceClient) Snapshot(ctx context.Context) (io.ReadCloser, error) {
	panic("TODO")
}

func (c *fakeMaintenanceClient) Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error) {
	panic("TODO")
}

func TestWriteSnap(t *testing.T) {
	// create fake maintenance client
	// use local file backend
	// Check file saved
	// check backup status not empty
}
