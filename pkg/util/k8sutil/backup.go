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
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/meta/metatypes"
	"k8s.io/kubernetes/pkg/api/resource"
	unversionedAPI "k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/apis/storage"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/util/intstr"
)

const (
	storageClassPrefix        = "etcd-operator-backup"
	BackupPodSelectorAppField = "etcd_backup_tool"
	backupPVVolName           = "etcd-backup-storage"
	awsCredentialDir          = "/root/.aws/"
	awsConfigDir              = "/root/.aws/config/"
	awsSecretVolName          = "secret-aws"
	awsConfigVolName          = "config-aws"
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
				"app":          "etcd",
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

	err = retryutil.Retry(4*time.Second, 5, func() (bool, error) {
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

func PodSpecWithPV(ps *api.PodSpec, clusterName string) *api.PodSpec {
	ps.Containers[0].VolumeMounts = []api.VolumeMount{{
		Name:      backupPVVolName,
		MountPath: constants.BackupDir,
	}}
	ps.Volumes = []api.Volume{{
		Name: backupPVVolName,
		VolumeSource: api.VolumeSource{
			PersistentVolumeClaim: &api.PersistentVolumeClaimVolumeSource{
				ClaimName: makePVCName(clusterName),
			},
		},
	}}
	return ps
}

func PodSpecWithS3(ps *api.PodSpec, s3Ctx s3config.S3Context) *api.PodSpec {
	ps.Containers[0].VolumeMounts = []api.VolumeMount{{
		Name:      awsSecretVolName,
		MountPath: awsCredentialDir,
	}, {
		Name:      awsConfigVolName,
		MountPath: awsConfigDir,
	}}
	ps.Volumes = []api.Volume{{
		Name: awsSecretVolName,
		VolumeSource: api.VolumeSource{
			Secret: &api.SecretVolumeSource{
				SecretName: s3Ctx.AWSSecret,
			},
		},
	}, {
		Name: awsConfigVolName,
		VolumeSource: api.VolumeSource{
			ConfigMap: &api.ConfigMapVolumeSource{
				LocalObjectReference: api.LocalObjectReference{
					Name: s3Ctx.AWSConfig,
				},
			},
		},
	}}
	ps.Containers[0].Env = append(ps.Containers[0].Env, api.EnvVar{
		Name:  backupenv.AWSConfig,
		Value: path.Join(awsConfigDir, "config"),
	}, api.EnvVar{
		Name:  backupenv.AWSS3Bucket,
		Value: s3Ctx.S3Bucket,
	})
	return ps
}

func MakeBackupPodSpec(clusterName string, policy *spec.BackupPolicy) (*api.PodSpec, error) {
	bp, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}

	ps := &api.PodSpec{
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
			},
		},
	}
	return ps, nil
}

func backupNameAndLabel(clusterName string) (string, map[string]string) {
	labels := map[string]string{
		"app":          BackupPodSelectorAppField,
		"etcd_cluster": clusterName,
	}
	name := MakeBackupName(clusterName)
	return name, labels
}

func MakeBackupReplicaSet(clusterName string, ps api.PodSpec, owner metatypes.OwnerReference) *extensions.ReplicaSet {
	name, labels := backupNameAndLabel(clusterName)
	rs := &extensions.ReplicaSet{
		ObjectMeta: api.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"etcd_cluster": clusterName,
				"app":          "etcd",
			},
		},
		Spec: extensions.ReplicaSetSpec{
			Replicas: 1,
			Selector: &unversionedAPI.LabelSelector{MatchLabels: labels},
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Labels: labels,
				},
				Spec: ps,
			},
		},
	}
	addOwnerRefToObject(rs.GetObjectMeta(), owner)
	return rs
}

func MakeBackupService(clusterName string, owner metatypes.OwnerReference) *api.Service {
	name, labels := backupNameAndLabel(clusterName)
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
	addOwnerRefToObject(svc.GetObjectMeta(), owner)
	return svc
}

func DeleteBackupReplicaSetAndService(kubecli *unversioned.Client, clusterName, ns string) error {
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
	return nil
}

func DeletePVC(kubecli *unversioned.Client, clusterName, ns string) error {
	return kubecli.PersistentVolumeClaims(ns).Delete(makePVCName(clusterName))
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

	// Delay could be very long due to k8s controller detaching the volume
	err := retryutil.Retry(10*time.Second, 12, func() (bool, error) {
		p, err := kubecli.Pods(ns).Get(pod.Name)
		if err != nil {
			return false, err
		}
		switch p.Status.Phase {
		case api.PodSucceeded:
			return true, nil
		case api.PodFailed:
			return false, fmt.Errorf("backup copy pod (%s) failed: %v, %v", pod.Name, pod.Status.Reason,
				pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Reason)
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("fail to wait backup copy pod (%s) succeeded: %v", pod.Name, err)
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
