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
	"github.com/coreos/etcd-operator/pkg/backup/s3/s3config"
	"github.com/coreos/etcd-operator/pkg/spec"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	v1beta1extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	v1beta1storage "k8s.io/client-go/pkg/apis/storage/v1beta1"
)

const (
	storageClassPrefix        = "etcd-backup"
	BackupPodSelectorAppField = "etcd_backup_tool"
	backupPVVolName           = "etcd-backup-storage"
	awsCredentialDir          = "/root/.aws/"
	awsConfigDir              = "/root/.aws/config/"
	awsSecretVolName          = "secret-aws"
	awsConfigVolName          = "config-aws"
	fromDirMountDir           = "/mnt/backup/from"

	PVBackupV1 = "v1" // TODO: refactor and combine this with pkg/backup.PVBackupV1
)

func CreateStorageClass(kubecli kubernetes.Interface, pvProvisioner string) error {
	// We need to get rid of prefix because naming doesn't support "/".
	name := storageClassPrefix + "-" + path.Base(pvProvisioner)
	class := &v1beta1storage.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: pvProvisioner,
	}
	_, err := kubecli.StorageV1beta1().StorageClasses().Create(class)
	return err
}

func CreateAndWaitPVC(kubecli kubernetes.Interface, clusterName, ns, pvProvisioner string, volumeSizeInMB int) error {
	name := makePVCName(clusterName)
	storageClassName := storageClassPrefix + "-" + path.Base(pvProvisioner)
	claim := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"etcd_cluster": clusterName,
				"app":          "etcd",
			},
			Annotations: map[string]string{
				"volume.beta.kubernetes.io/storage-class": storageClassName,
			},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dMi", volumeSizeInMB)),
				},
			},
		},
	}
	_, err := kubecli.CoreV1().PersistentVolumeClaims(ns).Create(claim)
	if err != nil {
		return err
	}

	// TODO: We set timeout to 60s here since PVC binding could take up to 60s for GCE/PD. See https://github.com/kubernetes/kubernetes/issues/40972 .
	//       Change the wait time once there are official p99 SLA.
	err = retryutil.Retry(4*time.Second, 15, func() (bool, error) {
		var err error
		claim, err = kubecli.CoreV1().PersistentVolumeClaims(ns).Get(name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if claim.Status.Phase != v1.ClaimBound {
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

func PodSpecWithPV(ps *v1.PodSpec, clusterName string) *v1.PodSpec {
	ps.Containers[0].VolumeMounts = []v1.VolumeMount{{
		Name:      backupPVVolName,
		MountPath: constants.BackupMountDir,
	}}
	ps.Volumes = []v1.Volume{{
		Name: backupPVVolName,
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: makePVCName(clusterName),
			},
		},
	}}
	return ps
}

func PodSpecWithS3(ps *v1.PodSpec, s3Ctx s3config.S3Context) *v1.PodSpec {
	ps.Containers[0].VolumeMounts = []v1.VolumeMount{{
		Name:      awsSecretVolName,
		MountPath: awsCredentialDir,
	}, {
		Name:      awsConfigVolName,
		MountPath: awsConfigDir,
	}}
	ps.Volumes = []v1.Volume{{
		Name: awsSecretVolName,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: s3Ctx.AWSSecret,
			},
		},
	}, {
		Name: awsConfigVolName,
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: s3Ctx.AWSConfig,
				},
			},
		},
	}}
	ps.Containers[0].Env = append(ps.Containers[0].Env, v1.EnvVar{
		Name:  backupenv.AWSConfig,
		Value: path.Join(awsConfigDir, "config"),
	}, v1.EnvVar{
		Name:  backupenv.AWSS3Bucket,
		Value: s3Ctx.S3Bucket,
	})
	return ps
}

func NewBackupPodSpec(clusterName, account string, sp spec.ClusterSpec) (*v1.PodSpec, error) {
	bp, err := json.Marshal(sp.Backup)
	if err != nil {
		return nil, err
	}

	var nsel map[string]string
	if sp.Pod != nil {
		nsel = sp.Pod.NodeSelector
	}

	ps := &v1.PodSpec{
		ServiceAccountName: account,
		NodeSelector:       nsel,
		Containers: []v1.Container{
			{
				Name:  "backup",
				Image: BackupImage,
				Command: []string{
					"/bin/sh",
					"-ec",
					"/usr/local/bin/etcd-backup --etcd-cluster=" + clusterName,
				},
				Env: []v1.EnvVar{{
					Name:      "MY_POD_NAMESPACE",
					ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
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
	name := BackupServiceName(clusterName)
	return name, labels
}

func NewBackupReplicaSetManifest(clusterName string, ps v1.PodSpec, owner metav1.OwnerReference) *v1beta1extensions.ReplicaSet {
	name, labels := backupNameAndLabel(clusterName)
	rs := &v1beta1extensions.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: newLablesForCluster(clusterName),
		},
		Spec: v1beta1extensions.ReplicaSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: ps,
			},
		},
	}
	addOwnerRefToObject(rs.GetObjectMeta(), owner)
	return rs
}

func NewBackupServiceManifest(clusterName string, owner metav1.OwnerReference) *v1.Service {
	name, labels := backupNameAndLabel(clusterName)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: newLablesForCluster(clusterName),
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "backup-service",
					Port:       constants.DefaultBackupPodHTTPPort,
					TargetPort: intstr.FromInt(constants.DefaultBackupPodHTTPPort),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
	addOwnerRefToObject(svc.GetObjectMeta(), owner)
	return svc
}

func DeletePVC(kubecli kubernetes.Interface, clusterName, ns string) error {
	err := kubecli.CoreV1().PersistentVolumeClaims(ns).Delete(makePVCName(clusterName), nil)
	if !IsKubernetesResourceNotFoundError(err) {
		return err
	}
	return nil
}

func CopyVolume(kubecli kubernetes.Interface, fromClusterName, toClusterName, ns string) error {
	from := path.Join(fromDirMountDir, PVBackupV1, fromClusterName)
	to := path.Join(constants.BackupMountDir, PVBackupV1, toClusterName)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: copyVolumePodName(toClusterName),
			Labels: map[string]string{
				"etcd_cluster": toClusterName,
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "copy-backup",
					Image: "alpine",
					Command: []string{
						"/bin/sh",
						"-ec",
						fmt.Sprintf("mkdir -p %[2]s; cp -r %[1]s/* %[2]s/", from, to),
					},
					VolumeMounts: []v1.VolumeMount{{
						Name:      "from-dir",
						MountPath: fromDirMountDir,
					}, {
						Name:      "to-dir",
						MountPath: constants.BackupMountDir,
					}},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
			Volumes: []v1.Volume{{
				Name: "from-dir",
				VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
						ClaimName: makePVCName(fromClusterName),
						ReadOnly:  true,
					},
				},
			}, {
				Name: "to-dir",
				VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
						ClaimName: makePVCName(toClusterName),
					},
				},
			}},
		},
	}
	if _, err := kubecli.CoreV1().Pods(ns).Create(pod); err != nil {
		return err
	}

	var phase v1.PodPhase
	// Delay could be very long due to k8s controller detaching the volume
	err := retryutil.Retry(10*time.Second, 12, func() (bool, error) {
		p, err := kubecli.CoreV1().Pods(ns).Get(pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		phase = p.Status.Phase
		switch p.Status.Phase {
		case v1.PodSucceeded:
			return true, nil
		case v1.PodFailed:
			var termReason string
			if len(pod.Status.ContainerStatuses) > 0 {
				termReason = pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Reason
			}
			return false, fmt.Errorf("backup copy pod (%s) failed: %v, %v", pod.Name, pod.Status.Reason, termReason)
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait backup copy pod (%s, phase: %s) to succeed: %v", pod.Name, phase, err)
	}
	// Delete the pod to detach the volume from the node
	return kubecli.CoreV1().Pods(ns).Delete(pod.Name, metav1.NewDeleteOptions(0))
}

func copyVolumePodName(clusterName string) string {
	return clusterName + "-copyvolume"
}

func makePVCName(clusterName string) string {
	return fmt.Sprintf("%s-pvc", clusterName)
}
