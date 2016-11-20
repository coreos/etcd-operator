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
	"time"

	"github.com/coreos/etcd-operator/pkg/util/constants"
	"github.com/coreos/etcd-operator/pkg/util/etcdutil"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
	"github.com/coreos/etcd/clientv3"
	"github.com/pborman/uuid"
)

func (c *Cluster) addOneSelfHostedMember() error {
	newMemberName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	c.idCounter++

	peerURL := "http://$(MY_POD_IP):2380"
	initialCluster := append(c.members.PeerURLPairs(), newMemberName+"="+peerURL)

	pod := k8sutil.MakeSelfHostedEtcdPod(newMemberName, initialCluster, c.name, "existing", "", c.spec)
	pod = k8sutil.PodWithAddMemberInitContainer(pod, c.members.ClientURLs(), newMemberName, []string{peerURL}, c.spec)

	_, err := c.kclient.Pods(c.namespace).Create(pod)
	if err != nil {
		return err
	}
	// wait for the new pod to start and add itself into the etcd cluster.
	cfg := clientv3.Config{
		Endpoints:   c.members.ClientURLs(),
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	oldN := c.members.Size()
	// TODO: do not wait forever.
	for {
		err = c.updateMembers(etcdcli)
		// TODO: check error type to determine the etcd member is not ready.
		if err != nil && c.members != nil {
			c.logger.Errorf("failed to list members for update: %v", err)
			return err
		}
		if c.members.Size() > oldN {
			break
		}
		time.Sleep(5 * time.Second)
	}

	c.logger.Infof("added a self-hosted member (%s)", newMemberName)
	return nil
}

func (c *Cluster) newSelfHostedSeedMember() error {
	newMemberName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	c.idCounter++
	initialCluster := []string{newMemberName + "=http://$(MY_POD_IP):2380"}

	pod := k8sutil.MakeSelfHostedEtcdPod(newMemberName, initialCluster, c.name, "new", uuid.New(), c.spec)
	_, err := k8sutil.CreateAndWaitPod(c.kclient, c.namespace, pod, 30*time.Second)
	if err != nil {
		return err
	}

	c.logger.Infof("self-hosted cluster created with seed member (%s)", newMemberName)
	return nil
}

func (c *Cluster) migrateBootMember() error {
	endpoint := c.spec.SelfHosted.BootMemberClientEndpoint

	c.logger.Infof("migrating boot member (%s)", endpoint)

	resp, err := etcdutil.ListMembers([]string{endpoint})
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
	newMemberName := fmt.Sprintf("%s-%04d", c.name, c.idCounter)
	c.idCounter++

	peerURL := "http://$(MY_POD_IP):2380"
	initialCluster = append(initialCluster, newMemberName+"="+peerURL)

	pod := k8sutil.MakeSelfHostedEtcdPod(newMemberName, initialCluster, c.name, "existing", "", c.spec)
	pod = k8sutil.PodWithAddMemberInitContainer(pod, []string{endpoint}, newMemberName, []string{peerURL}, c.spec)
	pod, err = k8sutil.CreateAndWaitPod(c.kclient, c.namespace, pod, 30*time.Second)
	if err != nil {
		return err
	}

	delay := 20 * time.Second
	c.logger.Infof("wait %v before removing the boot member", delay)
	time.Sleep(delay)

	err = removeMember([]string{"http://" + pod.Status.PodIP + ":2379"}, bootMember.ID)
	if err != nil {
		return fmt.Errorf("etcdcli failed to remove boot member: %v", err)
	}

	c.logger.Infof("self-hosted cluster created with boot member (%s)", endpoint)

	return nil
}
