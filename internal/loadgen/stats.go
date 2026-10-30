// Package loadgen measures latency and payload size. Load is open-loop:
// requests start on a fixed schedule whether or not earlier ones have
// finished, so a slow server cannot slow the load down and hide its own
// latency (coordinated omission).
package loadgen

import (
	"math"
	"slices"
	"time"
)

// Latency summarizes successful-call latencies in milliseconds.
type Latency struct {
	P50, P90, P99, P999, Max, Mean float64
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	return sorted[max(0, min(idx, len(sorted)-1))]
}

func summarize(samples []time.Duration) Latency {
	if len(samples) == 0 {
		return Latency{}
	}
	ms := make([]float64, len(samples))
	var sum float64
	for i, d := range samples {
		ms[i] = float64(d) / float64(time.Millisecond)
		sum += ms[i]
	}
	slices.Sort(ms)
	return Latency{
		P50: percentile(ms, 50), P90: percentile(ms, 90), P99: percentile(ms, 99),
		P999: percentile(ms, 99.9), Max: ms[len(ms)-1], Mean: sum / float64(len(ms)),
	}
}
