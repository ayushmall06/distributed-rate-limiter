package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_requests_total",
			Help: "Total Number of rate limit checks",
		},
		[]string{"tenant", "resource"},
	)

	AllowedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_allowed_total",
			Help: "Total Number of allowed requests",
		},
		[]string{"tenant", "resource"},
	)

	BlockedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_blocked_total",
			Help: "Total Number of blocked requests",
		},
		[]string{"tenant", "resource"},
	)

	ErrorsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "rate_limit_errors_total",
			Help: "Total Number of internal rate limiter errors",
		},
	)

	LatencyMs = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rate_limit_latency_ms",
			Help:    "Latency of rate limit checks in milliseconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"tenant", "resource"},
	)
)

func Register() {
	prometheus.MustRegister(
		RequestsTotal,
		AllowedTotal,
		BlockedTotal,
		ErrorsTotal,
		LatencyMs,
	)
}
