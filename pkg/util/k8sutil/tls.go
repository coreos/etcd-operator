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

package k8sutil

import (
	"github.com/coreos/etcd-operator/pkg/util/constants"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type OperatorSecretData struct {
	CertData []byte
	KeyData  []byte
	CAData   []byte
}

func GetTLSSecret(kubecli kubernetes.Interface, ns, se string) (*OperatorSecretData, error) {
	secret, err := kubecli.CoreV1().Secrets(ns).Get(se, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return &OperatorSecretData{
		CertData: secret.Data[constants.EtcdCliCertFile],
		KeyData:  secret.Data[constants.EtcdCliKeyFile],
		CAData:   secret.Data[constants.EtcdCliCAFile],
	}, nil
}
