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
