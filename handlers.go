package main

import (
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// profile describes a synthetic latency distribution. Jitter plus an occasional
// tail penalty are what make p95/p99 meaningful — a fixed sleep would give
// p50 == p95 == p99.
type profile struct {
	base        time.Duration
	jitter      time.Duration // uniform [0, jitter) added on top of base
	tailProb    float64       // fraction of requests that hit the long tail
	tailPenalty time.Duration // extra uniform [0, tailPenalty) when tail hits
}

var (
	// ~5-10ms: in-memory / cache hit.
	fastProfile = profile{base: 5 * time.Millisecond, jitter: 5 * time.Millisecond,
		tailProb: 0.02, tailPenalty: 20 * time.Millisecond}
	// ~50-80ms: a couple of DB/cache round-trips.
	averageProfile = profile{base: 50 * time.Millisecond, jitter: 30 * time.Millisecond,
		tailProb: 0.05, tailPenalty: 120 * time.Millisecond}
	// ~200-600ms: external call / heavy compute.
	slowProfile = profile{base: 200 * time.Millisecond, jitter: 100 * time.Millisecond,
		tailProb: 0.10, tailPenalty: 400 * time.Millisecond}
)

func (p profile) delay() time.Duration {
	d := p.base + time.Duration(rand.Int63n(int64(p.jitter)))
	if rand.Float64() < p.tailProb {
		d += time.Duration(rand.Int63n(int64(p.tailPenalty)))
	}
	return d
}

// handle turns a profile into an HTTP handler: sleep for the sampled delay,
// do a touch of CPU work so it isn't a pure sleep, then reply.
func handle(p profile) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		time.Sleep(p.delay())

		sink := 0.0
		for i := 0; i < 5000; i++ {
			sink += math.Sqrt(float64(i))
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"took_ms": time.Since(start).Milliseconds(),
			"sink":    sink,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// logging is a tiny access-log middleware (stdlib only).
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
