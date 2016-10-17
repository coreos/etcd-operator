// Copyright 2016 The kube-etcd-controller Authors
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

package chaos

import (
	"context"
	"math/rand"

	"github.com/Sirupsen/logrus"
	"golang.org/x/time/rate"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
)

// Monkeys knows how to crush pods and nodes.
type Monkeys struct {
	k8s *unversioned.Client
}

func NewMonkeys(k8s *unversioned.Client) *Monkeys {
	return &Monkeys{k8s: k8s}
}

// TODO: respect context in k8s operations.
func (m *Monkeys) CrushPods(ctx context.Context, ns string, ls labels.Selector, killRate float64) {
	limiter := rate.NewLimiter(rate.Limit(killRate), int(killRate))
	for {
		err := limiter.Wait(ctx)
		if err != nil { // user cancellation
			logrus.Infof("crushPods is cancelled for selector %v by the user: %v", ls.String(), err)
			return
		}

		pods, err := m.k8s.Pods(ns).List(api.ListOptions{LabelSelector: ls})
		if err != nil {
			logrus.Errorf("failed to list pods for selector %v: %v", ls.String(), err)
			continue
		}
		if len(pods.Items) == 0 {
			logrus.Infof("no pods to kill for selector %v", ls.String())
			continue
		}

		// todo: kill multiple pods in one round?
		tokill := pods.Items[rand.Intn(len(pods.Items))].Name
		err = m.k8s.Pods(ns).Delete(tokill, nil)
		if err != nil {
			logrus.Errorf("failed to kill pod %v: %v", tokill, err)
			continue
		}
		logrus.Infof("killed pod %v for selector %v", tokill, ls.String())
	}
}
