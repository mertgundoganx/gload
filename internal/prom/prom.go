package prom

import (
	"fmt"
	"net/http"
	"sync"
)

// Metrics holds Prometheus-compatible metrics for the gload instance.
type Metrics struct {
	mu sync.RWMutex

	// Gauges
	RunningTests  int
	TotalServices int

	// Counters (monotonically increasing)
	TestsCompleted int64
	TestsFailed    int64
	TotalRequests  int64
	TotalErrors    int64

	// Last test metrics (gauges)
	LastRPS        float64
	LastAvgLatency float64
	LastP95Latency float64
	LastErrorRate  float64
}

// Global is the singleton metrics instance used across the application.
var Global = &Metrics{}

// RecordTestComplete updates counters after a test finishes.
func (m *Metrics) RecordTestComplete(passed bool, totalReqs, errors int, rps, avgLat, p95Lat, errRate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TestsCompleted++
	if !passed {
		m.TestsFailed++
	}
	m.TotalRequests += int64(totalReqs)
	m.TotalErrors += int64(errors)
	m.LastRPS = rps
	m.LastAvgLatency = avgLat
	m.LastP95Latency = p95Lat
	m.LastErrorRate = errRate
}

// SetRunning updates the number of currently running tests.
func (m *Metrics) SetRunning(n int) {
	m.mu.Lock()
	m.RunningTests = n
	m.mu.Unlock()
}

// SetServices updates the total number of configured services.
func (m *Metrics) SetServices(n int) {
	m.mu.Lock()
	m.TotalServices = n
	m.mu.Unlock()
}

// Handler returns an http.HandlerFunc that serves Prometheus text format metrics.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := Global
		m.mu.RLock()
		defer m.mu.RUnlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		fmt.Fprintf(w, "# HELP gload_running_tests Number of currently running tests\n")
		fmt.Fprintf(w, "# TYPE gload_running_tests gauge\n")
		fmt.Fprintf(w, "gload_running_tests %d\n\n", m.RunningTests)

		fmt.Fprintf(w, "# HELP gload_total_services Total number of configured services\n")
		fmt.Fprintf(w, "# TYPE gload_total_services gauge\n")
		fmt.Fprintf(w, "gload_total_services %d\n\n", m.TotalServices)

		fmt.Fprintf(w, "# HELP gload_tests_completed_total Total tests completed\n")
		fmt.Fprintf(w, "# TYPE gload_tests_completed_total counter\n")
		fmt.Fprintf(w, "gload_tests_completed_total %d\n\n", m.TestsCompleted)

		fmt.Fprintf(w, "# HELP gload_tests_failed_total Total tests failed\n")
		fmt.Fprintf(w, "# TYPE gload_tests_failed_total counter\n")
		fmt.Fprintf(w, "gload_tests_failed_total %d\n\n", m.TestsFailed)

		fmt.Fprintf(w, "# HELP gload_requests_total Total HTTP requests made across all tests\n")
		fmt.Fprintf(w, "# TYPE gload_requests_total counter\n")
		fmt.Fprintf(w, "gload_requests_total %d\n\n", m.TotalRequests)

		fmt.Fprintf(w, "# HELP gload_errors_total Total HTTP errors across all tests\n")
		fmt.Fprintf(w, "# TYPE gload_errors_total counter\n")
		fmt.Fprintf(w, "gload_errors_total %d\n\n", m.TotalErrors)

		fmt.Fprintf(w, "# HELP gload_last_rps RPS of the most recent test\n")
		fmt.Fprintf(w, "# TYPE gload_last_rps gauge\n")
		fmt.Fprintf(w, "gload_last_rps %.2f\n\n", m.LastRPS)

		fmt.Fprintf(w, "# HELP gload_last_avg_latency_ms Average latency of the most recent test in ms\n")
		fmt.Fprintf(w, "# TYPE gload_last_avg_latency_ms gauge\n")
		fmt.Fprintf(w, "gload_last_avg_latency_ms %.2f\n\n", m.LastAvgLatency)

		fmt.Fprintf(w, "# HELP gload_last_p95_latency_ms P95 latency of the most recent test in ms\n")
		fmt.Fprintf(w, "# TYPE gload_last_p95_latency_ms gauge\n")
		fmt.Fprintf(w, "gload_last_p95_latency_ms %.2f\n\n", m.LastP95Latency)

		fmt.Fprintf(w, "# HELP gload_last_error_rate Error rate of the most recent test\n")
		fmt.Fprintf(w, "# TYPE gload_last_error_rate gauge\n")
		fmt.Fprintf(w, "gload_last_error_rate %.2f\n", m.LastErrorRate)
	}
}
