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
)

func init() {
	prometheus.MustRegister(clustersTotal)
	prometheus.MustRegister(clustersCreated)
	prometheus.MustRegister(clustersDeleted)
	prometheus.MustRegister(clustersModified)
}
