package main

import (
	"math"
	"sort"
)

type stats struct {
	n             int
	errors        int
	min, max      float64
	mean          float64
	p50, p95, p99 float64
}

// percentile returns the p-th percentile (0..100) of a sorted slice using the
// nearest-rank method.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

// compute derives summary statistics from a slice of latencies (ms). It sorts a
// copy, so the caller's slice is left untouched.
func compute(latsMs []float64, errs int) stats {
	s := stats{n: len(latsMs), errors: errs}
	if len(latsMs) == 0 {
		return s
	}
	sorted := append([]float64(nil), latsMs...)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	s.min = sorted[0]
	s.max = sorted[len(sorted)-1]
	s.mean = sum / float64(len(sorted))
	s.p50 = percentile(sorted, 50)
	s.p95 = percentile(sorted, 95)
	s.p99 = percentile(sorted, 99)
	return s
}
