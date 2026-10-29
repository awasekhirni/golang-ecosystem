// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package metrics defines the Prometheus collectors used across the service
// and exposes the canonical /metrics handler.
//
// Collectors:
//   - http_requests_total{method,route,status}  counter
//   - http_request_duration_seconds{method,route} histogram
//   - http_in_flight  gauge
//   - db_queries_total{op,entity}  counter
//   - db_query_duration_seconds{op,entity}  histogram
//   - domain_events_published_total{topic}  counter
//   - bus_dispatch_errors_total{topic,handler}  counter
//
// GAP CLOSED: #5 Prometheus metrics endpoint
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTP layer
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by method, route, status.",
	}, []string{"method", "route", "status"})

	HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})

	HTTPInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_in_flight",
		Help: "Currently in-flight HTTP requests.",
	})

	// DB layer
	DBQueries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "db_queries_total",
		Help: "Total DB queries by op and entity.",
	}, []string{"op", "entity"})

	DBQueryDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "db_query_duration_seconds",
		Help:    "DB query latency by op and entity.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"op", "entity"})

	// Domain events
	DomainEventsPublished = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "domain_events_published_total",
		Help: "Domain events published by topic.",
	}, []string{"topic"})

	BusDispatchErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "bus_dispatch_errors_total",
		Help: "Enterprise bus dispatch errors by topic and handler.",
	}, []string{"topic", "handler"})

	// Audit
	AuditEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "audit_events_total",
		Help: "Audit events by action.",
	}, []string{"action"})
)

func init() {
	prometheus.MustRegister(
		HTTPRequests, HTTPDuration, HTTPInFlight,
		DBQueries, DBQueryDuration,
		DomainEventsPublished, BusDispatchErrors,
		AuditEvents,
	)
}

// Handler returns the HTTP handler to mount at /metrics.
func Handler() http.Handler { return promhttp.Handler() }

// ObserveHTTP records one HTTP request. Called by the logger middleware.
func ObserveHTTP(method, route, status string, started time.Time) {
	HTTPRequests.WithLabelValues(method, route, status).Inc()
	HTTPDuration.WithLabelValues(method, route).Observe(time.Since(started).Seconds())
}

// ObserveDB records one DB query. Called by the enterprise repository base.
func ObserveDB(op, entity string, started time.Time) {
	DBQueries.WithLabelValues(op, entity).Inc()
	DBQueryDuration.WithLabelValues(op, entity).Observe(time.Since(started).Seconds())
}
