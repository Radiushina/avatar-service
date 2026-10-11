package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "avatars_http_requests_total",
			Help: "HTTP requests",
		},
		[]string{"method", "route", "status"},
	)

	HTTPDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "avatars_http_request_duration_seconds",
			Help:    "HTTP request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	DBConns = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "avatars_db_connections",
			Help: "Postgres pool connections",
		},
		[]string{"state"},
	)

	QueueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "avatars_queue_messages",
			Help: "RabbitMQ queue depth",
		},
		[]string{"queue"},
	)
)
