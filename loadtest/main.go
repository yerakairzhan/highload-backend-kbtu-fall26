package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// Load generator (stdlib only): fire N requests at randomly chosen endpoints
// through a fixed pool of workers, then hand the timings to the reporters in
// stats.go / report.go. Parameters come from config.json (see config.go);
// flags override individual values when given.
var endpoints = []string{"/fast", "/average", "/slow"}

type result struct {
	endpoint string
	latency  time.Duration
	status   int
	err      error
}

func main() {
	// Config file provides the defaults; flags can still override any of them.
	configPath := flag.String("config", "config.json", "path to JSON config file")
	cfgTmp, _ := loadConfig("config.json") // read once to seed flag defaults
	baseURL := flag.String("url", cfgTmp.URL, "server base URL")
	total := flag.Int("n", cfgTmp.Requests, "total number of requests")
	concurrency := flag.Int("c", cfgTmp.Concurrency, "concurrent workers")
	targetRPS := flag.Float64("target-rps", cfgTmp.TargetRPS, "target RPS for capacity estimation")
	duration := flag.Duration("duration", 0, "if >0, keep sending for this long instead of a fixed -n (e.g. 60s) — good for watching Grafana")
	flag.Parse()

	// If a non-default -config was passed, re-read it and refresh any flag the
	// user did NOT set explicitly on the command line.
	if *configPath != "config.json" {
		if cfg, err := loadConfig(*configPath); err == nil {
			set := map[string]bool{}
			flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
			if !set["url"] {
				*baseURL = cfg.URL
			}
			if !set["n"] {
				*total = cfg.Requests
			}
			if !set["c"] {
				*concurrency = cfg.Concurrency
			}
			if !set["target-rps"] {
				*targetRPS = cfg.TargetRPS
			}
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}

	jobs := make(chan string)
	results := make(chan result, *concurrency)

	// Worker pool. WaitGroup lets us close `results` once every worker is done,
	// so the collector works whether we send a fixed N or loop for a duration.
	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ep := range jobs {
				results <- doRequest(client, *baseURL, ep)
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()

	// Feeder: either loop until the deadline (duration mode) or send exactly N.
	wallStart := time.Now()
	go func() {
		defer close(jobs)
		if *duration > 0 {
			deadline := wallStart.Add(*duration)
			for time.Now().Before(deadline) {
				jobs <- endpoints[rand.Intn(len(endpoints))]
			}
			return
		}
		for i := 0; i < *total; i++ {
			jobs <- endpoints[rand.Intn(len(endpoints))]
		}
	}()

	if *duration > 0 {
		fmt.Printf("Sending continuous load for %s (watch Grafana)...\n", *duration)
	}

	perEndpoint := map[string][]float64{}
	perEndpointErrs := map[string]int{}
	var all []float64
	allErrs := 0
	for r := range results {
		if r.err != nil || r.status != http.StatusOK {
			perEndpointErrs[r.endpoint]++
			allErrs++
			continue
		}
		ms := float64(r.latency.Microseconds()) / 1000.0
		perEndpoint[r.endpoint] = append(perEndpoint[r.endpoint], ms)
		all = append(all, ms)
	}
	wall := time.Since(wallStart)

	epStats := map[string]stats{}
	for _, ep := range order {
		epStats[ep] = compute(perEndpoint[ep], perEndpointErrs[ep])
	}
	overall := compute(all, allErrs)

	actualTotal := len(all) + allErrs // duration mode sends an unknown count
	printLatencyTable(epStats, overall, actualTotal, *concurrency, *baseURL, wall)
	printCapacity(epStats, *targetRPS)
	printComparison(epStats)
}

func doRequest(client *http.Client, base, ep string) result {
	start := time.Now()
	resp, err := client.Get(base + ep)
	lat := time.Since(start)
	r := result{endpoint: ep, latency: lat, err: err}
	if err == nil {
		r.status = resp.StatusCode
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return r
}
