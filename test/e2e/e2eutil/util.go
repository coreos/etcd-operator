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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coreos/etcd-operator/client/experimentalclient"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

func DeleteSecrets(kubecli kubernetes.Interface, namespace string, secretNames ...string) error {
	var retErr error
	for _, sname := range secretNames {
		err := kubecli.CoreV1().Secrets(namespace).Delete(sname, metav1.NewDeleteOptions(0))
		if err != nil {
			retErr = fmt.Errorf("failed to delete secret (%s): %v; %v", sname, err, retErr)
		}
	}
	return retErr
}

func KillMembers(kubecli kubernetes.Interface, namespace string, names ...string) error {
	for _, name := range names {
		err := kubecli.CoreV1().Pods(namespace).Delete(name, metav1.NewDeleteOptions(0))
		if err != nil && !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func MakeBackup(kubecli kubernetes.Interface, ns, clusterName string) error {
	ls := labels.SelectorFromSet(k8sutil.BackupSidecarLabels(clusterName))
	podList, err := kubecli.CoreV1().Pods(ns).List(metav1.ListOptions{
		LabelSelector: ls.String(),
	})
	if err != nil {
		return err
	}
	if len(podList.Items) < 1 {
		return fmt.Errorf("no backup pod found")
	}

	// TODO: We are assuming Kubernetes pod network is accessible from test machine.
	addr := fmt.Sprintf("%s:%d", podList.Items[0].Status.PodIP, constants.DefaultBackupPodHTTPPort)
	bc := experimentalclient.NewBackupWithAddr(&http.Client{}, "http", addr)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = bc.Request(ctx)
	if err != nil {
		return fmt.Errorf("backup pod (%s): %v", podList.Items[0].Name, err)
	}
	return nil
}

func LogfWithTimestamp(t *testing.T, format string, args ...interface{}) {
	t.Log(time.Now(), fmt.Sprintf(format, args...))
}

func printContainerStatus(buf *bytes.Buffer, ss []v1.ContainerStatus) {
	for _, s := range ss {
		if s.State.Waiting != nil {
			buf.WriteString(fmt.Sprintf("%s: Waiting: message (%s) reason (%s)\n", s.Name, s.State.Waiting.Message, s.State.Waiting.Reason))
		}
		if s.State.Terminated != nil {
			buf.WriteString(fmt.Sprintf("%s: Terminated: message (%s) reason (%s)\n", s.Name, s.State.Terminated.Message, s.State.Terminated.Reason))
		}
	}
}
