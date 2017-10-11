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

package cluster

import (
	"github.com/prometheus/client_golang/prometheus"
)

var reconcileHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "etcd_operator",
	Subsystem: "cluster",
	Name:      "reconcile_duration",
	Help:      "Reconcile duration histogram in second",
	Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10),
},
	[]string{"ClusterName"},
)

var reconcileFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "etcd_operator",
	Subsystem: "cluster",
	Name:      "reconcile_failed",
	Help:      "Total number of failed reconcilations",
},
	[]string{"Reason"},
)

func init() {
	prometheus.MustRegister(reconcileHistogram)
	prometheus.MustRegister(reconcileFailed)
}
