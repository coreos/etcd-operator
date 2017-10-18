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

package controller

import (
	"fmt"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup"
	"github.com/coreos/etcd-operator/pkg/controller/controllerutil"

	"k8s.io/client-go/kubernetes"
)

// TODO: replace this with generic backend interface for other options (PV, Azure)
// handleS3 backups up etcd cluster to s3.
func handleS3(kubecli kubernetes.Interface, s3 *api.S3Source, namespace, clusterName string) error {
	be, err := controllerutil.NewS3backend(kubecli, s3, namespace, clusterName)
	if err != nil {
		return err
	}
	defer be.Close()
	bm := backup.NewBackupManager(kubecli, clusterName, namespace, nil, be)
	// this SaveSnap takes 0 as lastSnapRev to indicate that it
	// saves any snapshot with revision > 0.
	_, err = bm.SaveSnap(0)
	if err != nil {
		return fmt.Errorf("failed to save snapshot (%v)", err)
	}
	return nil
}
