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

package k8sutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	"k8s.io/kubernetes/pkg/client/unversioned"
)

var (
	ErrTPRObjectNotFound        = errors.New("TPR object not found")
	ErrTPRObjectVersionConflict = errors.New("TPR object's resource version conflicts")
)

func ListClusters(host, ns string, httpClient *http.Client) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters",
		host, ns))
}

func WatchClusters(host, ns string, httpClient *http.Client, resourceVersion string) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters?watch=true&resourceVersion=%s",
		host, ns, resourceVersion))
}

func WaitEtcdTPRReady(httpClient *http.Client, interval, timeout time.Duration, host, ns string) error {
	return retryutil.Retry(interval, int(timeout/interval), func() (bool, error) {
		resp, err := ListClusters(host, ns, httpClient)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			return true, nil
		case http.StatusNotFound: // not set up yet. wait.
			return false, nil
		default:
			return false, fmt.Errorf("invalid status code: %v", resp.Status)
		}
	})
}

func GetClusterTPRObject(k8s *unversioned.Client, host, ns, name string) (*spec.EtcdCluster, error) {
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters/%s", host, ns, name), nil)
	if err != nil {
		return nil, err
	}
	resp, err := k8s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	cluster := &spec.EtcdCluster{}
	if err := decoder.Decode(cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

// UpdateClusterTPRObject updates the given TPR object.
// ResourceVersion of the object MUST be set or update will fail.
func UpdateClusterTPRObject(k8s *unversioned.Client, host, ns string, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	if len(e.ResourceVersion) == 0 {
		return nil, errors.New("k8sutil: resource version is not provided")
	}
	return updateClusterTPRObject(k8s, host, ns, e)
}

// UpdateClusterTPRObjectUnconditionally updates the given TPR object.
// This should only be used in tests.
func UpdateClusterTPRObjectUnconditionally(k8s *unversioned.Client, host, ns string, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	e.ResourceVersion = ""
	return updateClusterTPRObject(k8s, host, ns, e)
}

func updateClusterTPRObject(k8s *unversioned.Client, host, ns string, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPut,
		fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters/%s", host, ns, e.Name),
		bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := k8s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrTPRObjectNotFound
	case http.StatusConflict:
		return nil, ErrTPRObjectVersionConflict
	default:
		return nil, fmt.Errorf("unexpected status: %v", resp.Status)
	}

	decoder := json.NewDecoder(resp.Body)
	cluster := &spec.EtcdCluster{}
	if err := decoder.Decode(cluster); err != nil {
		return nil, err
	}

	return cluster, nil
}
