package avatars

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	uploadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "avatars_uploads_total",
			Help: "Total number of avatar uploads",
		},
		[]string{"status"},
	)

	uploadDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "avatars_upload_duration_seconds",
			Help: "Avatar upload duration",
		},
		[]string{"status"},
	)

	storageUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "avatars_storage_bytes",
			Help: "Total storage used by avatars",
		},
		[]string{"user_id"},
	)
)
