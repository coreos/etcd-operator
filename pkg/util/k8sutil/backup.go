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
	"path"
	"time"

	backupenv "github.com/coreos/etcd-operator/pkg/backup/env"
	"github.com/coreos/etcd-operator/pkg/util/constants"

	"github.com/coreos/etcd-operator/pkg/spec"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/apis/storage"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/pkg/watch"
)

const (
	storageClassPrefix        = "etcd-operator-backup"
	BackupPodSelectorAppField = "etcd_backup_tool"
	fromDirMountDir           = "/mnt/backup/from"
)

func CreateStorageClass(kubecli *unversioned.Client, pvProvisioner string) error {
	// We need to get rid of prefix because naming doesn't support "/".
	name := storageClassPrefix + "-" + path.Base(pvProvisioner)
	class := &storage.StorageClass{
		ObjectMeta: api.ObjectMeta{
			Name: name,
		},
		Provisioner: pvProvisioner,
	}
	_, err := kubecli.StorageClasses().Create(class)
	return err
}

func CreateAndWaitPVC(kubecli *unversioned.Client, clusterName, ns, pvProvisioner string, volumeSizeInMB int) error {
	name := makePVCName(clusterName)
	storageClassName := storageClassPrefix + "-" + path.Base(pvProvisioner)
	claim := &api.PersistentVolumeClaim{
		ObjectMeta: api.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"etcd_cluster": clusterName,
			},
			Annotations: map[string]string{
				"volume.beta.kubernetes.io/storage-class": storageClassName,
			},
		},
		Spec: api.PersistentVolumeClaimSpec{
			AccessModes: []api.PersistentVolumeAccessMode{
				api.ReadWriteOnce,
			},
			Resources: api.ResourceRequirements{
				Requests: api.ResourceList{
					api.ResourceStorage: resource.MustParse(fmt.Sprintf("%dMi", volumeSizeInMB)),
				},
			},
		},
	}
	_, err := kubecli.PersistentVolumeClaims(ns).Create(claim)
	if err != nil {
		return err
	}

	err = wait.Poll(4*time.Second, 20*time.Second, func() (bool, error) {
		var err error
		claim, err = kubecli.PersistentVolumeClaims(ns).Get(name)
		if err != nil {
			return false, err
		}
		if claim.Status.Phase != api.ClaimBound {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		wErr := fmt.Errorf("fail to wait PVC (%s) '(%v)/Bound': %v", name, claim.Status.Phase, err)
		return wErr
	}

	return nil
}

var BackupImage = "quay.io/coreos/etcd-operator:latest"

func CreateBackupReplicaSetAndService(kubecli *unversioned.Client, clusterName, ns string, policy *spec.BackupPolicy) error {
	bp, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	labels := map[string]string{
		"app":          BackupPodSelectorAppField,
		"etcd_cluster": clusterName,
	}
	name := MakeBackupName(clusterName)
	_, err = kubecli.ReplicaSets(ns).Create(&extensions.ReplicaSet{
		ObjectMeta: api.ObjectMeta{
			Name: name,
		},
		Spec: extensions.ReplicaSetSpec{
			Replicas: 1,
			Selector: &unversionedAPI.LabelSelector{MatchLabels: labels},
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Labels: labels,
				},
				Spec: api.PodSpec{
					Containers: []api.Container{
						{
							Name:  "backup",
							Image: BackupImage,
							Command: []string{
								"/bin/sh",
								"-c",
								"/usr/local/bin/etcd-backup --etcd-cluster=" + clusterName,
							},
							Env: []api.EnvVar{{
								Name:      "MY_POD_NAMESPACE",
								ValueFrom: &api.EnvVarSource{FieldRef: &api.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
							}, {
								Name:  backupenv.BackupPolicy,
								Value: string(bp),
							}},
							VolumeMounts: []api.VolumeMount{{
								Name:      "etcd-backup-storage",
								MountPath: constants.BackupDir,
							}},
						},
					},
					Volumes: []api.Volume{{
						Name: "etcd-backup-storage",
						VolumeSource: api.VolumeSource{
							PersistentVolumeClaim: &api.PersistentVolumeClaimVolumeSource{
								ClaimName: makePVCName(clusterName),
							},
						},
					}},
				},
			},
		},
	})
	if err != nil {
		if !IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}

	svc := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: api.ServiceSpec{
			Ports: []api.ServicePort{
				{
					Name:       "backup-service",
					Port:       constants.DefaultBackupPodHTTPPort,
					TargetPort: intstr.FromInt(constants.DefaultBackupPodHTTPPort),
					Protocol:   api.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
	if _, err := kubecli.Services(ns).Create(svc); err != nil {
		if !IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	return nil
}

func DeleteBackupReplicaSetAndService(kubecli *unversioned.Client, clusterName, ns string, cleanup bool) error {
	name := MakeBackupName(clusterName)
	err := kubecli.Services(ns).Delete(name)
	if err != nil {
		return err
	}
	orphanOption := false
	gracePeriod := int64(0)
	err = kubecli.ReplicaSets(ns).Delete(name, &api.DeleteOptions{
		OrphanDependents:   &orphanOption,
		GracePeriodSeconds: &gracePeriod,
	})
	if err != nil {
		return err
	}
	if cleanup {
		kubecli.PersistentVolumeClaims(ns).Delete(makePVCName(clusterName))
	}
	return nil
}

func CopyVolume(kubecli *unversioned.Client, fromClusterName, toClusterName, ns string) error {
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: copyVolumePodName(toClusterName),
			Labels: map[string]string{
				"etcd_cluster": toClusterName,
			},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Name:  "copy-backup",
					Image: "alpine",
					Command: []string{
						"/bin/sh",
						"-c",
						fmt.Sprintf("cp -r %s/* %s/", fromDirMountDir, constants.BackupDir),
					},
					VolumeMounts: []api.VolumeMount{{
						Name:      "from-dir",
						MountPath: fromDirMountDir,
					}, {
						Name:      "to-dir",
						MountPath: constants.BackupDir,
					}},
				},
			},
			RestartPolicy: api.RestartPolicyNever,
			Volumes: []api.Volume{{
				Name: "from-dir",
				VolumeSource: api.VolumeSource{
					PersistentVolumeClaim: &api.PersistentVolumeClaimVolumeSource{
						ClaimName: makePVCName(fromClusterName),
						ReadOnly:  true,
					},
				},
			}, {
				Name: "to-dir",
				VolumeSource: api.VolumeSource{
					PersistentVolumeClaim: &api.PersistentVolumeClaimVolumeSource{
						ClaimName: makePVCName(toClusterName),
					},
				},
			}},
		},
	}
	if _, err := kubecli.Pods(ns).Create(pod); err != nil {
		return err
	}

	w, err := kubecli.Pods(ns).Watch(api.SingleObject(api.ObjectMeta{Name: pod.Name}))
	if err != nil {
		return err
	}
	// It could take long due to delay of k8s controller detaching the volume
	_, err = watch.Until(120*time.Second, w, unversioned.PodCompleted)
	if err != nil {
		return fmt.Errorf("fail to wait data copy job completed: %v", err)
	}
	// Delete the pod to detach the volume from the node
	return kubecli.Pods(ns).Delete(pod.Name, api.NewDeleteOptions(0))
}

func copyVolumePodName(clusterName string) string {
	return clusterName + "-copyvolume"
}

func makePVCName(clusterName string) string {
	return fmt.Sprintf("%s-pvc", clusterName)
}
