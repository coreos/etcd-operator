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

import "github.com/prometheus/client_golang/prometheus"

var (
	clustersTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "etcd_operator",
		Subsystem: "controller",
		Name:      "clusters",
		Help:      "Total number of clusters managed by the controller",
	})

	clustersCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "etcd_operator",
		Subsystem: "controller",
		Name:      "clusters_created",
		Help:      "Total number of clusters created",
	})

	clustersDeleted = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "etcd_operator",
		Subsystem: "controller",
		Name:      "clusters_deleted",
		Help:      "Total number of clusters deleted",
	})

	clustersModified = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "etcd_operator",
		Subsystem: "controller",
		Name:      "clusters_modified",
		Help:      "Total number of clusters modified",
	})

	clustersFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "etcd_operator",
		Subsystem: "controller",
		Name:      "clusters_failed",
		Help:      "Total number of clusters failed",
	})
)

func init() {
	prometheus.MustRegister(clustersTotal)
	prometheus.MustRegister(clustersCreated)
	prometheus.MustRegister(clustersDeleted)
	prometheus.MustRegister(clustersModified)
	prometheus.MustRegister(clustersFailed)
}
