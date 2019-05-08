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
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/backup/reader"
	"github.com/coreos/etcd-operator/pkg/util/alibabacloudutil/ossfactory"
	"github.com/coreos/etcd-operator/pkg/util/awsutil/s3factory"
	"github.com/coreos/etcd-operator/pkg/util/azureutil/absfactory"
	"github.com/coreos/etcd-operator/pkg/util/gcputil/gcsfactory"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	backupHTTPPath = backupapi.APIV1 + "/backup/"
	listenAddr     = "0.0.0.0:19999"
)

func (r *Restore) startHTTP() {
	http.HandleFunc(backupapi.APIV1+"/backup/", r.handleServeBackup)
	logrus.Infof("listening on %v", listenAddr)
	panic(http.ListenAndServe(listenAddr, nil))
}

func (r *Restore) handleServeBackup(w http.ResponseWriter, req *http.Request) {
	err := r.serveBackup(w, req)
	if err != nil {
		logrus.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// serveBackup parses incoming request url of the form /backup/<restore-name>
// get the etcd restore name.
// Then it returns the etcd cluster backup snapshot to the caller.
func (r *Restore) serveBackup(w http.ResponseWriter, req *http.Request) error {
	restoreName := string(req.URL.Path[len(backupHTTPPath):])
	if len(restoreName) == 0 {
		return errors.New("restore name is not specified")
	}

	obj := &api.EtcdRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreName,
			Namespace: r.namespace,
		},
	}
	v, exists, err := r.indexer.Get(obj)
	if err != nil {
		return fmt.Errorf("failed to get restore CR for restore-name (%v): %v", restoreName, err)
	}
	if !exists {
		return fmt.Errorf("no restore CR found for restore-name (%v)", restoreName)
	}

	logrus.Infof("serving backup for restore CR %v", restoreName)
	cr := v.(*api.EtcdRestore)

	var (
		backupReader reader.Reader
		path         string
	)

	switch cr.Spec.BackupStorageType {
	case api.BackupStorageTypeS3:
		restoreSource := cr.Spec.RestoreSource
		if restoreSource.S3 == nil {
			return errors.New("empty s3 restore source")
		}
		s3RestoreSource := restoreSource.S3
		if len(s3RestoreSource.AWSSecret) == 0 || len(s3RestoreSource.Path) == 0 {
			return errors.New("invalid s3 restore source field (spec.s3), must specify all required subfields")
		}

		s3Cli, err := s3factory.NewClientFromSecret(r.kubecli, r.namespace, s3RestoreSource.Endpoint, s3RestoreSource.AWSSecret, s3RestoreSource.ForcePathStyle)
		if err != nil {
			return fmt.Errorf("failed to create S3 client: %v", err)
		}
		defer s3Cli.Close()

		backupReader = reader.NewS3Reader(s3Cli.S3)
		path = s3RestoreSource.Path
	case api.BackupStorageTypeABS:
		restoreSource := cr.Spec.RestoreSource
		if restoreSource.ABS == nil {
			return errors.New("empty abs restore source")
		}
		absRestoreSource := restoreSource.ABS
		if len(absRestoreSource.ABSSecret) == 0 || len(absRestoreSource.Path) == 0 {
			return errors.New("invalid abs restore source field (spec.abs), must specify all required subfields")
		}

		absCli, err := absfactory.NewClientFromSecret(r.kubecli, r.namespace, absRestoreSource.ABSSecret)
		if err != nil {
			return fmt.Errorf("failed to create ABS client: %v", err)
		}
		// Nothing to Close for absCli yet

		backupReader = reader.NewABSReader(absCli.ABS)
		path = absRestoreSource.Path
	case api.BackupStorageTypeGCS:
		ctx := context.TODO()
		restoreSource := cr.Spec.RestoreSource
		if restoreSource.GCS == nil {
			return errors.New("empty gcs restore source")
		}
		gcsRestoreSource := restoreSource.GCS
		if len(gcsRestoreSource.Path) == 0 {
			return errors.New("invalid gcs restore source field (spec.gcs), must specify all required subfields")
		}

		gcsCli, err := gcsfactory.NewClientFromSecret(ctx, r.kubecli, r.namespace, gcsRestoreSource.GCPSecret)
		if err != nil {
			return fmt.Errorf("failed to create GCS client: %v", err)
		}
		defer gcsCli.GCS.Close()

		backupReader = reader.NewGCSReader(ctx, gcsCli.GCS)
		path = gcsRestoreSource.Path
	case api.BackupStorageTypeOSS:
		restoreSource := cr.Spec.RestoreSource
		if restoreSource.OSS == nil {
			return errors.New("empty oss restore source")
		}
		ossRestoreSource := restoreSource.OSS
		if len(ossRestoreSource.OSSSecret) == 0 || len(ossRestoreSource.Path) == 0 {
			return errors.New("invalid oss restore source field (spec.oss), must specify all required subfields")
		}

		ossCli, err := ossfactory.NewClientFromSecret(r.kubecli, r.namespace, ossRestoreSource.Endpoint, ossRestoreSource.OSSSecret)
		if err != nil {
			return fmt.Errorf("failed to create OSS client: %v", err)
		}

		backupReader = reader.NewOSSReader(ossCli.OSS)
		path = ossRestoreSource.Path
	default:
		return fmt.Errorf("unknown backup storage type (%s) for restore CR (%v)", cr.Spec.BackupStorageType, restoreName)
	}

	rc, err := backupReader.Open(path)
	if err != nil {
		return fmt.Errorf("failed to read backup file(%v): %v", path, err)
	}
	defer rc.Close()

	_, err = io.Copy(w, rc)
	if err != nil {
		return fmt.Errorf("failed to write backup to %s: %v", req.RemoteAddr, err)
	}
	return nil
}
