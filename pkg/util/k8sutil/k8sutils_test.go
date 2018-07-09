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
	"testing"

	"fmt"
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"net/url"
	"os"
)

func TestDefaultBusyboxImageName(t *testing.T) {
	policy := &api.PodPolicy{}
	image := imageNameBusybox(policy)
	expected := defaultBusyboxImage
	if image != expected {
		t.Errorf("expect image=%s, get=%s", expected, image)
	}
}

func TestDefaultNilBusyboxImageName(t *testing.T) {
	image := imageNameBusybox(nil)
	expected := defaultBusyboxImage
	if image != expected {
		t.Errorf("expect image=%s, get=%s", expected, image)
	}
}

func TestSetBusyboxImageName(t *testing.T) {
	policy := &api.PodPolicy{
		BusyboxImage: "myRepo/busybox:1.3.2",
	}
	image := imageNameBusybox(policy)
	expected := "myRepo/busybox:1.3.2"
	if image != expected {
		t.Errorf("expect image=%s, get=%s", expected, image)
	}
}

func TestMakeRestoreInitContainers(t *testing.T) {
	testCases := []struct {
		backupURL         *url.URL
		token             string
		repo              string
		version           string
		m                 *etcdutil.Member
		expectedCurlImage string
		setup             func()
		teardown          func()
	}{
		{
			backupURL: &url.URL{
				Scheme: "http",
				Host:   "test",
				Path:   "etcd",
			},
			token:   "1234",
			repo:    "test_repo",
			version: "1",
			m: &etcdutil.Member{
				Name:      "member-test",
				Namespace: "default",
				ID:        0,
			},
			setup: func() {
				os.Setenv("CUSTOM_CURL_IMAGE", "awesome/curl")
			},
			teardown: func() {
				os.Unsetenv("CUSTOM_CURL_IMAGE")
			},
			expectedCurlImage: "awesome/curl",
		},
		{
			backupURL: &url.URL{
				Scheme: "http",
				Host:   "test1",
				Path:   "etcd1",
			},
			token:   "4321",
			repo:    "repo_test",
			version: "2",
			m: &etcdutil.Member{
				Name:         "member-test1",
				Namespace:    "kube-system",
				ID:           1,
				SecurePeer:   true,
				SecureClient: true,
			},
			expectedCurlImage: "tutum/curl",
			setup: func() {
			},
			teardown: func() {
			},
		},
	}
	for testCaseIndex := range testCases {
		testCases[testCaseIndex].setup()
		v1Container := makeRestoreInitContainers(testCases[testCaseIndex].backupURL, testCases[testCaseIndex].token, testCases[testCaseIndex].repo, testCases[testCaseIndex].version, testCases[testCaseIndex].m)
		assert.Equal(t, len(v1Container), 2)

		backupContainer := v1Container[0]
		restoreContainer := v1Container[1]

		expectedBackupContainer := v1.Container{
			Name:  "fetch-backup",
			Image: testCases[testCaseIndex].expectedCurlImage,
			Command: []string{
				"/bin/bash", "-ec",
				fmt.Sprintf(`
httpcode=$(curl --write-out %%\{http_code\} --silent --output %[1]s %[2]s)
if [[ "$httpcode" != "200" ]]; then
	echo "http status code: ${httpcode}" >> /dev/termination-log
	cat %[1]s >> /dev/termination-log
	exit 1
fi
					`, backupFile, testCases[testCaseIndex].backupURL.String()),
			},
			VolumeMounts: etcdVolumeMounts(),
		}

		assert.Equal(t, backupContainer, expectedBackupContainer)

		expectedRestoreContainer := v1.Container{
			Name:  "restore-datadir",
			Image: ImageName(testCases[testCaseIndex].repo, testCases[testCaseIndex].version),
			Command: []string{
				"/bin/sh", "-ec",
				fmt.Sprintf("ETCDCTL_API=3 etcdctl snapshot restore %[1]s"+
					" --name %[2]s"+
					" --initial-cluster %[2]s=%[3]s"+
					" --initial-cluster-token %[4]s"+
					" --initial-advertise-peer-urls %[3]s"+
					" --data-dir %[5]s 2>/dev/termination-log", backupFile, testCases[testCaseIndex].m.Name, testCases[testCaseIndex].m.PeerURL(), testCases[testCaseIndex].token, dataDir),
			},
			VolumeMounts: etcdVolumeMounts(),
		}

		assert.Equal(t, restoreContainer, expectedRestoreContainer)
		testCases[testCaseIndex].teardown()
	}
}