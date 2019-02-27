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

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	watchtools "k8s.io/client-go/tools/watch"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "kube config file path")
	e2ePodName := flag.String("e2e-podname", "", "e2e test pod's name")
	ns := flag.String("namespace", "default", "e2e test namespace")
	logsDir := flag.String("logs-dir", "_output/logs/", "dir for pods' logs")
	flag.Parse()

	if len(*e2ePodName) == 0 {
		panic("must set --e2e-podname")
	}

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	kubecli, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	stopCh := make(chan struct{})
	podListWatcher := cache.NewListWatchFromClient(kubecli.CoreV1().RESTClient(), "pods", *ns, fields.Everything())
	_, informer := cache.NewIndexerInformer(podListWatcher, &v1.Pod{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			go func(pod *v1.Pod, namespace, logsDir string) {
				watcher, err := kubecli.CoreV1().Pods(namespace).Watch(metav1.SingleObject(metav1.ObjectMeta{Name: pod.Name}))
				if err != nil {
					logrus.Errorf("failed to watch for pod (%s): %v", pod.Name, err)
					return
				}
				ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), 100*time.Second)
				defer cancel()
				// We need to wait Pod Running before collecting its log
				ev, err := watchtools.UntilWithoutRetry(ctx, watcher, podRunning)
				if err != nil {
					if err == watchtools.ErrWatchClosed {
						// present a consistent error interface to callers
						err = wait.ErrWaitTimeout
					}
					logrus.Errorf("failed to reach Running for pod (%s): %v", pod.Name, err)
					return
				}
				pod = ev.Object.(*v1.Pod)
				containers := append(pod.Spec.InitContainers, pod.Spec.Containers...)
				for i := range containers {
					go func(c v1.Container) {
						logOption := &v1.PodLogOptions{Follow: true, Container: c.Name}
						req := kubecli.CoreV1().Pods(namespace).GetLogs(pod.Name, logOption)
						readCloser, err := req.Stream()
						if err != nil {
							logrus.Errorf("failed to open stream for pod (%s): %v", pod.Name, err)
							return
						}
						defer readCloser.Close()

						f, err := os.OpenFile(filepath.Join(logsDir, pod.Name+"."+c.Name+".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
						if err != nil {
							logrus.Errorf("failed to open log file for pod (%s/%s): %v", pod.Name, c.Name, err)
							return
						}
						_, err = io.Copy(f, readCloser)
						if err != nil {
							logrus.Errorf("failed to write log for pod (%s/%s): %v", pod.Name, c.Name, err)
						}
					}(containers[i])
				}
			}(obj.(*v1.Pod), *ns, *logsDir)
		},
		UpdateFunc: func(old, new interface{}) {
			pod := new.(*v1.Pod)
			if pod.Name != *e2ePodName {
				return
			}
			// If e2e test pod runs to completion, then stops this program.
			if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed || pod.DeletionTimestamp != nil {
				close(stopCh)
			}
		},
	}, cache.Indexers{})

	logrus.Info("start collecting logs...")
	informer.Run(stopCh)
	logrus.Info("stop collecting logs...")
}

// **NOTE**: Copy from kubernetes.
// podRunning returns true if the pod is running, false if the pod has not yet reached running state,
// returns ErrPodCompleted if the pod has run to completion, or an error in any other case.
func podRunning(event watch.Event) (bool, error) {
	switch event.Type {
	case watch.Deleted:
		return false, errors.New("pod deleted")
	}
	switch t := event.Object.(type) {
	case *v1.Pod:
		switch t.Status.Phase {
		case v1.PodRunning:
			return true, nil
		case v1.PodFailed, v1.PodSucceeded:
			return false, errors.New("pod ran to completion")
		}
	}
	return false, nil
}
