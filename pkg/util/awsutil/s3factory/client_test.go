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

package s3factory

import (
	"testing"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSetupAWSConfig(t *testing.T) {
	sec := &v1.Secret{
		Data: map[string][]byte{},
	}

	client := fake.NewSimpleClientset(sec)

	e := "example.com"
	opts, err := setupAWSConfig(client, "", "", e, "")
	if err != nil {
		t.Error(err)
	}

	if e != *opts.Config.Endpoint {
		t.Errorf("got: %s wanted: %s", *opts.Config.Endpoint, e)
	}
}

func TestS3ClientForIAMRoles(t *testing.T) {
	tests := []struct {
		name   string
		cfg    ClientConfig
		expErr bool
	}{
		{
			name: "A configuration without AWS secret, should return a client without credentials.",
			cfg: ClientConfig{
				Endpoint: "s3.eu-west-1.amazonaws.com",
			},
			expErr: false,
		},
		{
			name: "A configuration without AWS secret and bad endpoint without region, should return a error.",
			cfg: ClientConfig{
				Endpoint: "s3.amazonaws.com",
			},
			expErr: true,
		},
		{
			name:   "A configuration without AWS secret and no endpoint, should return a error.",
			cfg:    ClientConfig{},
			expErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			client := fake.NewSimpleClientset()
			_, err := NewClient(test.cfg, client)

			if test.expErr && err == nil {
				t.Errorf("error expected, dind't got error")
			} else if !test.expErr && err != nil {
				t.Errorf("error not expected got: %s", err)
			}
		})
	}
}
