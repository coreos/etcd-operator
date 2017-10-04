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

package debug

import (
	"os"
	"path"

	"github.com/coreos/etcd-operator/pkg/util/k8sutil"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
)

var (
	// This flag should be set to enable debug logging
	DebugFilePath string
)

type DebugLogger struct {
	// regular log to stdout
	logger *logrus.Entry
	// log to file for debugging self hosted clusters
	fileLogger *logrus.Logger
}

func New(clusterName string) *DebugLogger {
	if len(DebugFilePath) == 0 {
		return nil
	}

	logger := logrus.WithField("pkg", "debug")
	err := os.MkdirAll(path.Dir(DebugFilePath), 0755)
	if err != nil {
		logger.Errorf("Could not create debug log directory (%v), debug logging will not be performed: %v", path.Dir(DebugFilePath), err)
		return nil
	}

	logFile, err := os.OpenFile(DebugFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logger.Errorf("failed to open debug log file(%v): %v", DebugFilePath, err)
		return nil
	}

	l := logrus.New()
	l.Out = logFile
	l.Infof("Starting debug logs for self-hosted etcd cluster: %v", clusterName)
	return &DebugLogger{
		logger:     logrus.WithField("pkg", "debug"),
		fileLogger: l,
	}
}

func (dl *DebugLogger) LogPodCreation(pod *v1.Pod) {
	podSpec, err := k8sutil.PodSpecToPrettyJSON(pod)
	if err != nil {
		dl.fileLogger.Infof("failed to get readable spec for pod(%v): %v ", pod.Name, err)
	}
	dl.fileLogger.Infof("created pod (%s) with spec: %s\n", pod.Name, podSpec)
}

func (dl *DebugLogger) LogPodDeletion(podName string) {
	dl.fileLogger.Infof("deleted pod (%s)", podName)
}

func (dl *DebugLogger) LogClusterSpecUpdate(oldSpec, newSpec string) {
	dl.fileLogger.Infof("spec update: \nOld:\n%v \nNew:\n%v\n", oldSpec, newSpec)
}

func (dl *DebugLogger) LogMessage(msg string) {
	dl.fileLogger.Infof(msg)
}
