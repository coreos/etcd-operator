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
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GregoryIan/operator/pkg/spec"
	"github.com/GregoryIan/operator/pkg/util"
	"github.com/GregoryIan/operator/pkg/util/constants"
	"github.com/GregoryIan/operator/pkg/util/etcdutil"

	"github.com/juju/errors"
	"k8s.io/kubernetes/pkg/api"
	apierrors "k8s.io/kubernetes/pkg/api/errors"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
	k8sv1api "k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/pkg/watch"
)

func MustGetInClusterMasterHost() string {
	cfg, err := restclient.InClusterConfig()
	if err != nil {
		panic(err)
	}
	return cfg.Host
}

// tlsConfig isn't modified inside this function.
// The reason it's a pointer is that it's not necessary to have tlsconfig to create a client.
func MustCreateClient(host string, tlsInsecure bool, tlsConfig *restclient.TLSClientConfig) *unversioned.Client {
	if len(host) == 0 {
		c, err := unversioned.NewInCluster()
		if err != nil {
			panic(err)
		}
		return c
	}
	cfg := &restclient.Config{
		Host:  host,
		QPS:   100,
		Burst: 100,
	}
	hostUrl, err := url.Parse(host)
	if err != nil {
		panic(fmt.Sprintf("error parsing host url %s : %v", host, err))
	}
	if hostUrl.Scheme == "https" {
		cfg.TLSClientConfig = *tlsConfig
		cfg.Insecure = tlsInsecure
	}
	c, err := unversioned.New(cfg)
	if err != nil {
		panic(err)
	}
	return c
}

func IsKubernetesResourceAlreadyExistError(err error) bool {
	se, ok := err.(*apierrors.StatusError)
	if !ok {
		return false
	}
	if se.Status().Code == http.StatusConflict && se.Status().Reason == unversionedAPI.StatusReasonAlreadyExists {
		return true
	}
	return false
}

func IsKubernetesResourceNotFoundError(err error) bool {
	se, ok := err.(*apierrors.StatusError)
	if !ok {
		return false
	}
	if se.Status().Code == http.StatusNotFound && se.Status().Reason == unversionedAPI.StatusReasonNotFound {
		return true
	}
	return false
}

func ListTiDBCluster(host, ns string, httpClient *http.Client) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/pingcap.com/v1/namespaces/%s/tidbclusters",
		host, ns))
}

func WatchTiDBCluster(host, ns string, httpClient *http.Client, resourceVersion string) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/pingcap.com/v1/namespaces/%s/tidbclusters?watch=true&resourceVersion=%s",
		host, ns, resourceVersion))
}

func WaitTiDBTPRReady(httpClient *http.Client, interval, timeout time.Duration, host, ns string) error {
	return wait.Poll(interval, timeout, func() (bool, error) {
		resp, err := ListTiDBCluster(host, ns, httpClient)
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

func PodListOpt(clusterName string, op ServiceType) api.ListOptions {
	return api.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"tidb_cluster": clusterName,
			"app":          op.String(),
		}),
	}
}
