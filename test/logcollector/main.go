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
	"flag"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
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
			pod := obj.(*v1.Pod)
			go func(namespace, logsDir string) {
				// TODO: wait until pod is running
				time.Sleep(5 * time.Second)

				req := kubecli.CoreV1().Pods(namespace).GetLogs(pod.Name, &v1.PodLogOptions{Follow: true})
				readCloser, err := req.Stream()
				if err != nil {
					logrus.Errorf("failed to open stream for pod (%s): %v", pod.Name, err)
					return
				}
				defer readCloser.Close()

				f, err := os.OpenFile(filepath.Join(logsDir, pod.Name+".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					logrus.Errorf("failed to open log file for pod (%s): %v", pod.Name, err)
					return
				}
				_, err = io.Copy(f, readCloser)
				if err != nil {
					logrus.Errorf("failed to write log for pod (%s): %v", pod.Name, err)
				}
			}(*ns, *logsDir)
		},
		UpdateFunc: func(old, new interface{}) {
			pod := new.(*v1.Pod)
			if pod.Name != *e2ePodName {
				return
			}
			if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
				close(stopCh)
			}
		},
		DeleteFunc: nil,
	}, cache.Indexers{})

	logrus.Info("start collecting logs...")
	informer.Run(stopCh)
	logrus.Info("start collecting logs...")
}
