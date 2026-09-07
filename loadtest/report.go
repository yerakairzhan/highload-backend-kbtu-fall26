package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// order fixes the display order of endpoints in every table.
var order = []string{"/fast", "/average", "/slow"}

func printLatencyTable(epStats map[string]stats, overall stats, total int, concurrency int, url string, wall time.Duration) {
	fmt.Printf("\n=== Load test: %d requests, concurrency=%d, %s ===\n", total, concurrency, url)
	fmt.Printf("Wall clock: %.2fs   Achieved throughput: %.1f req/s\n\n",
		wall.Seconds(), float64(total)/wall.Seconds())

	header := fmt.Sprintf("%-10s %6s %6s %8s %8s %8s %8s %8s %8s",
		"endpoint", "count", "errs", "min", "mean", "p50", "p95", "p99", "max")
	fmt.Println(header)
	fmt.Println(strings.Repeat("-", len(header)))

	for _, ep := range order {
		printRow(ep, epStats[ep])
	}
	fmt.Println(strings.Repeat("-", len(header)))
	printRow("ALL", overall)
}

func printRow(name string, s stats) {
	fmt.Printf("%-10s %6d %6d %7.1f %7.1f %7.1f %7.1f %7.1f %7.1f\n",
		name, s.n, s.errors, s.min, s.mean, s.p50, s.p95, s.p99, s.max)
}

// printCapacity sizes worker pools against p95 latency using Little's Law:
// required concurrency = target_RPS x latency(seconds).
func printCapacity(epStats map[string]stats, targetRPS float64) {
	fmt.Printf("\n=== Capacity estimation (target = %.0f req/s) ===\n", targetRPS)
	fmt.Println("Model: Little's Law -> required in-flight concurrency = RPS x latency(s).")
	fmt.Print("We size against p95 (SLO-driven) so the tail doesn't starve the pool.\n\n")

	fmt.Printf("%-10s %10s %14s %18s\n", "endpoint", "p95(ms)", "1-worker RPS", "workers @ target")
	fmt.Println(strings.Repeat("-", 54))
	for _, ep := range order {
		s := epStats[ep]
		if s.n == 0 {
			continue
		}
		lat := s.p95 / 1000.0 // seconds
		fmt.Printf("%-10s %10.1f %14.1f %18.0f\n",
			ep, s.p95, 1.0/lat, math.Ceil(targetRPS*lat))
	}
	fmt.Println("\nReading: 'workers @ target' = concurrent request slots needed to sustain")
	fmt.Println("the target RPS at p95 latency. Divide by cores/instance to get instance count.")
}

func printComparison(epStats map[string]stats) {
	f, a, sl := epStats["/fast"], epStats["/average"], epStats["/slow"]
	fmt.Println("\n=== Comparison ===")
	if f.n > 0 && a.n > 0 {
		fmt.Printf("- /average p95 is %.1fx /fast p95.\n", a.p95/f.p95)
	}
	if f.n > 0 && sl.n > 0 {
		fmt.Printf("- /slow p95 is %.1fx /fast p95.\n", sl.p95/f.p95)
		fmt.Printf("- Tail spread: /slow p99/p50 = %.2f vs /fast p99/p50 = %.2f (higher = fatter tail).\n",
			sl.p99/sl.p50, f.p99/f.p50)
	}
	fmt.Println("- To hit the same RPS, /slow needs the most workers -> it dominates capacity cost.")
}
