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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
)

// TODO: replace this package with Operator client

func WatchClusters(host, ns string, httpClient *http.Client, resourceVersion string) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/%s/%s/namespaces/%s/clusters?watch=true&resourceVersion=%s",
		host, spec.TPRGroup, spec.TPRVersion, ns, resourceVersion))
}

func GetClusterList(restcli rest.Interface, ns string) (*spec.ClusterList, error) {
	b, err := restcli.Get().RequestURI(listClustersURI(ns)).DoRaw()
	if err != nil {
		return nil, err
	}

	clusters := &spec.ClusterList{}
	if err := json.Unmarshal(b, clusters); err != nil {
		return nil, err
	}
	return clusters, nil
}

func WaitEtcdTPRReady(restcli rest.Interface, interval, timeout time.Duration, ns string) error {
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
	return fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters", spec.TPRGroup, spec.TPRVersion, ns)
}

func GetClusterTPRObject(restcli rest.Interface, ns, name string) (*spec.Cluster, error) {
	uri := fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters/%s", spec.TPRGroup, spec.TPRVersion, ns, name)
	b, err := restcli.Get().RequestURI(uri).DoRaw()
	if err != nil {
		return nil, err
	}
	return readOutCluster(b)
}

// UpdateClusterTPRObject updates the given TPR object.
// ResourceVersion of the object MUST be set or update will fail.
func UpdateClusterTPRObject(restcli rest.Interface, ns string, c *spec.Cluster) (*spec.Cluster, error) {
	if len(c.Metadata.ResourceVersion) == 0 {
		return nil, errors.New("k8sutil: resource version is not provided")
	}
	return updateClusterTPRObject(restcli, ns, c)
}

// UpdateClusterTPRObjectUnconditionally updates the given TPR object.
// This should only be used in tests.
func UpdateClusterTPRObjectUnconditionally(restcli rest.Interface, ns string, c *spec.Cluster) (*spec.Cluster, error) {
	c.Metadata.ResourceVersion = ""
	return updateClusterTPRObject(restcli, ns, c)
}

func updateClusterTPRObject(restcli rest.Interface, ns string, c *spec.Cluster) (*spec.Cluster, error) {
	uri := fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters/%s", spec.TPRGroup, spec.TPRVersion, ns, c.Metadata.Name)
	b, err := restcli.Put().RequestURI(uri).Body(c).DoRaw()
	if err != nil {
		return nil, err
	}
	return readOutCluster(b)
}

func readOutCluster(b []byte) (*spec.Cluster, error) {
	cluster := &spec.Cluster{}
	if err := json.Unmarshal(b, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}
