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

// SetStorageUsage replaces avatars_storage_bytes with the current totals.
// ponytail: the server recounts SUM(size_bytes) every 15s. A just-finished upload shows up on the next tick; Add/Sub on the write path would be immediate.
func SetStorageUsage(usage map[string]float64) {
	storageUsage.Reset()
	for userID, bytes := range usage {
		storageUsage.WithLabelValues(userID).Set(bytes)
	}
}
