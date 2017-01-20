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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	apierrors "k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/rest"
)

// TODO: replace this package with Operator client

func WatchClusters(host, ns string, httpClient *http.Client, resourceVersion string) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/coreos.com/v1/namespaces/%s/etcdclusters?watch=true&resourceVersion=%s",
		host, ns, resourceVersion))
}

func GetClusterList(restcli *rest.RESTClient, ns string) (*spec.EtcdClusterList, error) {
	b, err := restcli.Get().RequestURI(listClustersURI(ns)).DoRaw()
	if err != nil {
		return nil, err
	}

	clusters := &spec.EtcdClusterList{}
	if err := json.Unmarshal(b, clusters); err != nil {
		return nil, err
	}
	return clusters, nil
}

func WaitEtcdTPRReady(restcli *rest.RESTClient, interval, timeout time.Duration, ns string) error {
	return retryutil.Retry(interval, int(timeout/interval), func() (bool, error) {
		_, err := restcli.Get().RequestURI(listClustersURI(ns)).DoRaw()
		if err != nil {
			if apierrors.IsNotFound(err) { // not set up yet. wait more.
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func listClustersURI(ns string) string {
	return fmt.Sprintf("/apis/coreos.com/v1/namespaces/%s/etcdclusters", ns)
}

func GetClusterTPRObject(restcli *rest.RESTClient, ns, name string) (*spec.EtcdCluster, error) {
	uri := fmt.Sprintf("/apis/coreos.com/v1/namespaces/%s/etcdclusters/%s", ns, name)
	b, err := restcli.Get().RequestURI(uri).DoRaw()
	if err != nil {
		return nil, err
	}
	return readOutCluster(b)
}

// UpdateClusterTPRObject updates the given TPR object.
// ResourceVersion of the object MUST be set or update will fail.
func UpdateClusterTPRObject(restcli *rest.RESTClient, ns string, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	if len(e.ResourceVersion) == 0 {
		return nil, errors.New("k8sutil: resource version is not provided")
	}
	return updateClusterTPRObject(restcli, ns, e)
}

// UpdateClusterTPRObjectUnconditionally updates the given TPR object.
// This should only be used in tests.
func UpdateClusterTPRObjectUnconditionally(restcli *rest.RESTClient, ns string, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	e.ResourceVersion = ""
	return updateClusterTPRObject(restcli, ns, e)
}

func updateClusterTPRObject(restcli *rest.RESTClient, ns string, e *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	uri := fmt.Sprintf("/apis/coreos.com/v1/namespaces/%s/etcdclusters/%s", ns, e.Name)
	b, err := restcli.Put().RequestURI(uri).Body(e).DoRaw()
	if err != nil {
		return nil, err
	}
	return readOutCluster(b)
}

func readOutCluster(b []byte) (*spec.EtcdCluster, error) {
	cluster := &spec.EtcdCluster{}
	if err := json.Unmarshal(b, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}
