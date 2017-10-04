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

package backuputil

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"path"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coreos/etcd/clientv3"
	"k8s.io/api/core/v1"
)

func NewAWSConfig(secret *v1.Secret, tmpDir string) (*session.Options, error) {
	dir, err := ioutil.TempDir(tmpDir, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create dir %v: (%v)", dir, err)
	}

	options := &session.Options{}
	options.SharedConfigState = session.SharedConfigEnable

	creds := secret.Data[api.AWSSecretCredentialsFileName]
	if len(creds) != 0 {
		credsFile := path.Join(dir, "credentials")
		err := ioutil.WriteFile(credsFile, creds, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to write credentials file: (%v)", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, credsFile)
	}

	config := secret.Data[api.AWSSecretConfigFileName]
	if config != nil {
		configFile := path.Join(dir, "config")
		err := ioutil.WriteFile(configFile, config, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to write config file: (%v)", err)
		}
		options.SharedConfigFiles = append(options.SharedConfigFiles, configFile)
	}
	return options, nil
}

func GetMemberWithMaxRev(pods []*v1.Pod, tc *tls.Config) (*etcdutil.Member, int64) {
	var member *etcdutil.Member
	maxRev := int64(0)
	for _, pod := range pods {
		m := &etcdutil.Member{
			Name:         pod.Name,
			Namespace:    pod.Namespace,
			SecureClient: tc != nil,
		}
		cfg := clientv3.Config{
			Endpoints:   []string{m.ClientURL()},
			DialTimeout: constants.DefaultDialTimeout,
			TLS:         tc,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			logrus.Warningf("failed to create etcd client for pod %v: (%v)", pod.Name, err)
			continue
		}
		defer etcdcli.Close()
		ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
		resp, err := etcdcli.Get(ctx, "/", clientv3.WithSerializable())
		cancel()
		if err != nil {
			logrus.Warningf("getMaxRev: failed to get revision from member %s (%s)", m.Name, m.ClientURL())
			continue
		}
		logrus.Infof("getMaxRev: member %s revision (%d)", m.Name, resp.Header.Revision)
		if resp.Header.Revision > maxRev {
			maxRev = resp.Header.Revision
			member = m
		}
	}
	return member, maxRev
}
