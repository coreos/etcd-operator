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

package cluster

import (
	"fmt"
	"math"
	"time"

	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd-operator/pkg/util/retryutil"

	"github.com/pborman/uuid"
)

func (c *Cluster) addOneSelfHostedMember() error {
	c.status.AppendScalingUpCondition(c.members.Size(), c.cluster.Spec.Size)

	newMemberName := etcdutil.CreateMemberName(c.cluster.Metadata.Name, c.memberCounter)
	c.memberCounter++

	peerURL := fmt.Sprintf("=https://%s.%s.svc.cluster.local:2380", newMemberName, c.cluster.Metadata.Namespace)
	initialCluster := append(c.members.PeerURLPairs(), newMemberName+"="+peerURL)

	pod := k8sutil.NewSelfHostedEtcdPod(newMemberName, initialCluster, c.cluster.Metadata.Name, c.cluster.Metadata.Namespace, "existing", "", c.cluster.Spec, c.cluster.AsOwner())

	pod = k8sutil.PodWithAddMemberInitContainer(pod, c.members.ClientURLs(), newMemberName, []string{peerURL}, c.cluster.Spec)

	_, err := c.config.KubeCli.CoreV1().Pods(c.cluster.Metadata.Namespace).Create(pod)
	if err != nil {
		return err
	}
	// wait for the new pod to start and add itself into the etcd cluster.
	oldN := c.members.Size()
	err = retryutil.Retry(5*time.Second, math.MaxInt64, func() (bool, error) {
		err = c.updateMembers(c.members)
		if err != nil {
			c.logger.Errorf("add self hosted member: fail to update members: %v", err)
			if err == errUnexpectedUnreadyMember {
				return false, nil
			}
			return false, err
		}
		if c.members.Size() > oldN {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	c.logger.Infof("added a self-hosted member (%s)", newMemberName)
	return nil
}

func (c *Cluster) newSelfHostedSeedMember() error {
	newMemberName := fmt.Sprintf("%s-%04d", c.cluster.Metadata.Name, c.memberCounter)
	c.memberCounter++
	initialCluster := []string{fmt.Sprintf("%s=https://%s.%s.svc.cluster.local:2380", newMemberName, newMemberName, c.cluster.Metadata.Namespace)}

	pod := k8sutil.NewSelfHostedEtcdPod(newMemberName, initialCluster, c.cluster.Metadata.Name, c.cluster.Metadata.Namespace, "new", uuid.New(), c.cluster.Spec, c.cluster.AsOwner())
	_, err := k8sutil.CreateAndWaitPod(c.config.KubeCli, c.cluster.Metadata.Namespace, pod, 30*time.Second)

	if err != nil {
		return err
	}

	c.logger.Infof("self-hosted cluster created with seed member (%s)", newMemberName)
	return nil
}

func (c *Cluster) migrateBootMember() error {
	endpoint := c.cluster.Spec.SelfHosted.BootMemberClientEndpoint

	c.logger.Infof("migrating boot member (%s)", endpoint)

	resp, err := etcdutil.ListMembers(c.etcdTLSConfig, []string{endpoint})
	if err != nil {
		return fmt.Errorf("failed to list members from boot member (%v)", err)
	}
	if len(resp.Members) != 1 {
		return fmt.Errorf("boot cluster contains more than one member")
	}
	bootMember := resp.Members[0]

	initialCluster := make([]string, 0)
	for _, purl := range bootMember.PeerURLs {
		initialCluster = append(initialCluster, fmt.Sprintf("%s=%s", bootMember.Name, purl))
	}

	// create the  member inside Kubernetes for migration
	newMemberName := fmt.Sprintf("%s-%04d", c.cluster.Metadata.Name, c.memberCounter)
	c.memberCounter++

	peerURL := fmt.Sprintf("https://%s.%s.svc.cluster.local:2380", newMemberName, c.cluster.Metadata.Namespace)
	initialCluster = append(initialCluster, newMemberName+"="+peerURL)

	pod := k8sutil.NewSelfHostedEtcdPod(newMemberName, initialCluster, c.cluster.Metadata.Name, c.cluster.Metadata.Namespace, "existing", "", c.cluster.Spec, c.cluster.AsOwner())

	pod = k8sutil.PodWithAddMemberInitContainer(pod, []string{endpoint}, newMemberName, []string{peerURL}, c.cluster.Spec)
	pod, err = k8sutil.CreateAndWaitPod(c.config.KubeCli, c.cluster.Metadata.Namespace, pod, 30*time.Second)
	if err != nil {
		return err
	}

	go func() {
		// TODO: a shorter timeout?
		// Waiting here for cluster to get stable:
		// - etcd data are replicated;
		// - cluster TPR state has switched to "Running"
		delay := 60 * time.Second
		c.logger.Infof("wait %v before removing the boot member", delay)
		time.Sleep(delay)

		err = etcdutil.RemoveMember(c.etcdTLSConfig, []string{fmt.Sprintf("https://%s.%s.svc.cluster.local:2379", newMemberName, c.cluster.Metadata.Namespace)}, bootMember.ID)
		if err != nil {
			c.logger.Errorf("boot member migration: failed to remove the boot member (%v)", err)
		}
	}()

	c.logger.Infof("self-hosted cluster created with boot member (%s)", endpoint)

	return nil
}
