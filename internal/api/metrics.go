package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	BytesDownloadedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "glutton_bytes_downloaded_total",
			Help: "Total bytes drained per source (labels: source).",
		},
		[]string{"source"},
	)
	CurrentRateGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "glutton_current_rate_bps",
			Help: "Current downstream rate in bytes per second.",
		},
	)
	ActiveWorkers = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "glutton_active_workers",
			Help: "Number of active worker goroutines.",
		},
	)
	SourceRTTSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "glutton_source_rtt_seconds",
			Help:    "Time-to-first-byte per source.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"source"},
	)
)

func init() {
	prometheus.MustRegister(BytesDownloadedTotal, CurrentRateGauge, ActiveWorkers, SourceRTTSeconds)
}

// metricsHandler returns the standard promhttp handler.
var metricsHandler = promhttp.Handler()
