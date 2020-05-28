package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	BackupsAttemptedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "etcd_operator",
		Name:      "backups_attempt_total",
		Help:      "Backups attempt by name and namespace",
	},
		[]string{"name", "namespace"},
	)

	BackupsSuccessTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "etcd_operator",
		Name:      "backups_success_total",
		Help:      "Backups success by name and namespace",
	},
		[]string{"name", "namespace"},
	)

	BackupsLastSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "etcd_operator",
		Name:      "backup_last_success",
		Help:      "Timestamp of last successfull backup, by name and namespace",
	},
		[]string{"name", "namespace"},
	)
)

func init() {
	prometheus.MustRegister(BackupsAttemptedTotal)
	prometheus.MustRegister(BackupsSuccessTotal)
	prometheus.MustRegister(BackupsLastSuccess)
}
