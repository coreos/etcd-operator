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
	"time"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// TODO: replace this package with Operator client

// EtcdClusterCRUpdateFunc is a function to be used when atomically
// updating a Cluster CR.
type EtcdClusterCRUpdateFunc func(*spec.EtcdCluster)

func WatchClusters(host, ns string, httpClient *http.Client, resourceVersion string) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/%s/namespaces/%s/%s?watch=true&resourceVersion=%s",
		host, spec.SchemeGroupVersion.String(), ns, spec.CRDResourcePlural, resourceVersion))
}

func GetClusterList(restcli rest.Interface, ns string) (*spec.EtcdClusterList, error) {
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

func listClustersURI(ns string) string {
	return fmt.Sprintf("/apis/%s/namespaces/%s/%s", spec.SchemeGroupVersion.String(), ns, spec.CRDResourcePlural)
}

func GetClusterTPRObject(restcli rest.Interface, ns, name string) (*spec.EtcdCluster, error) {
	uri := fmt.Sprintf("/apis/%s/namespaces/%s/%s/%s", spec.SchemeGroupVersion.String(), ns, spec.CRDResourcePlural, name)
	b, err := restcli.Get().RequestURI(uri).DoRaw()
	if err != nil {
		return nil, err
	}
	return readClusterCR(b)
}

// AtomicUpdateClusterTPRObject will get the latest result of a cluster,
// let user modify it, and update the cluster with modified result
// The entire process would be retried if there is a conflict of resource version
func AtomicUpdateClusterTPRObject(restcli rest.Interface, name, namespace string, maxRetries int, updateFunc EtcdClusterCRUpdateFunc) (*spec.EtcdCluster, error) {
	var updatedCluster *spec.EtcdCluster
	err := retryutil.Retry(1*time.Second, maxRetries, func() (done bool, err error) {
		currCluster, err := GetClusterTPRObject(restcli, namespace, name)
		if err != nil {
			return false, err
		}

		updateFunc(currCluster)

		updatedCluster, err = UpdateClusterTPRObject(restcli, namespace, currCluster)
		if err != nil {
			if apierrors.IsConflict(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	return updatedCluster, err
}

func UpdateClusterTPRObject(restcli rest.Interface, ns string, c *spec.EtcdCluster) (*spec.EtcdCluster, error) {
	uri := fmt.Sprintf("/apis/%s/namespaces/%s/%s/%s", spec.SchemeGroupVersion.String(), ns, spec.CRDResourcePlural, c.Name)
	b, err := restcli.Put().RequestURI(uri).Body(c).DoRaw()
	if err != nil {
		return nil, err
	}
	return readClusterCR(b)
}

func readClusterCR(b []byte) (*spec.EtcdCluster, error) {
	cluster := &spec.EtcdCluster{}
	if err := json.Unmarshal(b, cluster); err != nil {
		return nil, fmt.Errorf("read cluster CR from json data failed: %v", err)
	}
	return cluster, nil
}

func CreateCRD(clientset apiextensionsclient.Interface) error {
	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: spec.CRDName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   spec.SchemeGroupVersion.Group,
			Version: spec.SchemeGroupVersion.Version,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural:     spec.CRDResourcePlural,
				Kind:       spec.CRDResourceKind,
				ShortNames: []string{"etcd"},
			},
		},
	}
	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	return err
}

func WaitCRDReady(clientset apiextensionsclient.Interface) error {
	err := retryutil.Retry(5*time.Second, 20, func() (bool, error) {
		crd, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Get(spec.CRDName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range crd.Status.Conditions {
			switch cond.Type {
			case apiextensionsv1beta1.Established:
				if cond.Status == apiextensionsv1beta1.ConditionTrue {
					return true, nil
				}
			case apiextensionsv1beta1.NamesAccepted:
				if cond.Status == apiextensionsv1beta1.ConditionFalse {
					return false, fmt.Errorf("Name conflict: %v", cond.Reason)
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("wait CRD created failed: %v", err)
	}
	return nil
}

func MustNewKubeExtClient() apiextensionsclient.Interface {
	cfg, err := InClusterConfig()
	if err != nil {
		panic(err)
	}
	return apiextensionsclient.NewForConfigOrDie(cfg)
}
