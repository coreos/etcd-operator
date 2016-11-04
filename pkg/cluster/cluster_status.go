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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	k8sclient "k8s.io/kubernetes/pkg/client/unversioned"
)

type Status struct {
	CurrentVersion string `json:"currentVersion"`
	TargetVersion  string `json:"targetVersion"`
}

func (s *Status) upgradeVersionTo(v string) {
	s.TargetVersion = v
}

func (s *Status) setVersion(v string) {
	s.TargetVersion = ""
	s.CurrentVersion = v
}

type StatusTPR struct {
	unversioned.TypeMeta `json:",inline"`
	api.ObjectMeta       `json:"metadata,omitempty"`
	Status               Status `json:"status"`
}

func (s *Status) reportToTPR(k8s *k8sclient.Client, host, ns, name string) error {
	if !ReportStatusToTPR {
		return nil
	}

	st := &StatusTPR{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "EtcdClusterStatus",
			APIVersion: "coreos.com/v1",
		},
		ObjectMeta: api.ObjectMeta{
			Name: name,
		},
		Status: *s,
	}

	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	contentType := "application/json"
	p := fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclustersstatus/%s", host, ns, name)

	// try update first
	req, err := http.NewRequest("PUT", p, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := k8s.Client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected status: %v", resp.Status)
	}

	// create if not found
	resp, err = k8s.Client.Post(p, contentType, bytes.NewReader(b))
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %v", resp.Status)
	}
	return nil
}
