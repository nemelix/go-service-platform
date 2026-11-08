package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by status code and method.",
	}, []string{"code", "method"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"code", "method"})

	visitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "visits_total",
		Help: "Visits recorded by this process.",
	})
)

func instrument(next http.Handler) http.Handler {
	return promhttp.InstrumentHandlerDuration(httpDuration,
		promhttp.InstrumentHandlerCounter(httpRequests, next))
}
