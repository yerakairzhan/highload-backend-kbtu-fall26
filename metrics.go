package main

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Minimal, dependency-free Prometheus histogram exporter.
//
// This records latency for the SYSTEM AS A WHOLE — every request, regardless of
// endpoint, lands in the same histogram. Grafana then draws system-wide
// p50/p95/p99 and total throughput. No per-endpoint labels, no client library.

// bucketBounds are the "less-than-or-equal" upper edges in SECONDS, ascending.
// They span ~5ms (fast path) up to ~1s (slow tail) so quantiles are accurate.
var bucketBounds = []float64{
	0.005, 0.01, 0.025, 0.05, 0.075,
	0.1, 0.15, 0.2, 0.3, 0.4, 0.5, 0.75, 1.0,
}

type histogram struct {
	mu     sync.Mutex
	counts []uint64 // per-bucket count (len == len(bucketBounds))
	sum    float64  // sum of all latencies (seconds)
	total  uint64   // total observations (the +Inf bucket)
}

func newHistogram() *histogram {
	return &histogram{counts: make([]uint64, len(bucketBounds))}
}

var metrics = newHistogram()

// observe records one request latency (seconds) into the system histogram.
func (h *histogram) observe(seconds float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Bump the single smallest bucket this sample fits in. Samples larger than
	// the last bound fall into the implicit +Inf bucket. Buckets are made
	// cumulative on export (Prometheus requires cumulative le).
	for i, b := range bucketBounds {
		if seconds <= b {
			h.counts[i]++
			break
		}
	}
	h.sum += seconds
	h.total++
}

// ServeHTTP writes the Prometheus exposition format (system-wide, unlabeled).
func (h *histogram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	const name = "http_request_duration_seconds"
	fmt.Fprintf(w, "# HELP %s System-wide HTTP request latency in seconds.\n", name)
	fmt.Fprintf(w, "# TYPE %s histogram\n", name)

	cum := uint64(0)
	for i, b := range bucketBounds {
		cum += h.counts[i]
		le := strconv.FormatFloat(b, 'g', -1, 64)
		fmt.Fprintf(w, "%s_bucket{le=%q} %d\n", name, le, cum)
	}
	fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", name, h.total)
	fmt.Fprintf(w, "%s_sum %g\n", name, h.sum)
	fmt.Fprintf(w, "%s_count %d\n", name, h.total)
}

// measure wraps a handler so every call is timed and recorded system-wide.
func measure(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next(w, r)
		metrics.observe(time.Since(start).Seconds())
	}
}
