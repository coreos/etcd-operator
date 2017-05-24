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

package e2eutil

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/aws/aws-sdk-go/service/s3"
	"k8s.io/client-go/kubernetes"
)

type StorageCheckerOptions struct {
	S3Cli    *s3.S3
	S3Bucket string
}

func CreateCluster(t *testing.T, kubeClient kubernetes.Interface, namespace string, cl *spec.Cluster) (*spec.Cluster, error) {
	uri := fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters", spec.TPRGroup, spec.TPRVersion, namespace)
	b, err := kubeClient.CoreV1().RESTClient().Post().Body(cl).RequestURI(uri).DoRaw()
	if err != nil {
		return nil, err
	}
	res := &spec.Cluster{}
	if err := json.Unmarshal(b, res); err != nil {
		return nil, err
	}
	LogfWithTimestamp(t, "created etcd cluster: %v", res.Metadata.Name)
	return res, nil
}

func UpdateCluster(kubeClient kubernetes.Interface, cl *spec.Cluster, maxRetries int, updateFunc k8sutil.ClusterTPRUpdateFunc) (*spec.Cluster, error) {
	return k8sutil.AtomicUpdateClusterTPRObject(kubeClient.CoreV1().RESTClient(), cl.Metadata.Name, cl.Metadata.Namespace, maxRetries, updateFunc)
}

func DeleteCluster(t *testing.T, kubeClient kubernetes.Interface, cl *spec.Cluster) error {
	uri := fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters/%s", spec.TPRGroup, spec.TPRVersion, cl.Metadata.Namespace, cl.Metadata.Name)
	if _, err := kubeClient.CoreV1().RESTClient().Delete().RequestURI(uri).DoRaw(); err != nil {
		return err
	}
	return waitResourcesDeleted(t, kubeClient, cl)
}

func DeleteClusterAndBackup(t *testing.T, kubecli kubernetes.Interface, cl *spec.Cluster, checkerOpt StorageCheckerOptions) error {
	err := DeleteCluster(t, kubecli, cl)
	if err != nil {
		return err
	}
	err = WaitBackupDeleted(kubecli, cl, checkerOpt)
	if err != nil {
		return fmt.Errorf("fail to wait backup deleted: %v", err)
	}
	return nil
}
