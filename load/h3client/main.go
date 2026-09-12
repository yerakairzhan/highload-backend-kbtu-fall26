// Command h3client is the HTTP/3 tool for the Week 2 rig, plus an honest
// version prover for all three protocols.
//
// Why Go and not curl: there is no arm64 image of a curl built with HTTP/3, so
// on an Apple-silicon host the curl-h3 route does not exist. quic-go is
// arm64-native, matches the course's Go stack, and reports the negotiated
// protocol (res.Proto) just as reliably as curl's %{http_version}. State this
// substitution in REPORT.md.
//
// Modes:
//
//	-mode prove  one request, print the negotiated protocol + phase timings
//	-mode load   closed-loop load (like h2load -n N -c C), print percentiles
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func clientFor(proto string) (*http.Client, func()) {
	tlsCfg := &tls.Config{InsecureSkipVerify: true} // lab only
	switch proto {
	case "h3":
		tr := &http3.Transport{TLSClientConfig: tlsCfg}
		return &http.Client{Transport: tr, Timeout: 5 * time.Second}, func() { _ = tr.Close() }
	case "h2":
		tr := &http.Transport{TLSClientConfig: tlsCfg, ForceAttemptHTTP2: true}
		return &http.Client{Transport: tr, Timeout: 5 * time.Second}, tr.CloseIdleConnections
	default: // h1
		tr := &http.Transport{
			TLSClientConfig: tlsCfg,
			// Empty, non-nil map disables the automatic HTTP/2 upgrade, so
			// ALPN genuinely negotiates http/1.1.
			TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
		}
		return &http.Client{Transport: tr, Timeout: 5 * time.Second}, tr.CloseIdleConnections
	}
}

func prove(proto, url string) error {
	client, closeFn := clientFor(proto)
	defer closeFn()

	var connectDone, tlsDone, firstByte time.Time
	start := time.Now()
	trace := &httptrace.ClientTrace{
		ConnectDone:          func(_, _ string, _ error) { connectDone = time.Now() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tlsDone = time.Now() },
		GotFirstResponseByte: func() { firstByte = time.Now() },
	}
	req, _ := http.NewRequestWithContext(
		httptrace.WithClientTrace(context.Background(), trace), http.MethodGet, url, nil)

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", proto, err)
	}
	defer res.Body.Close()
	// Count body bytes: a 200 with an empty body means the edge answered but
	// never proxied (e.g. a Host mismatch). That must fail loudly, not pass.
	n, _ := io.Copy(io.Discard, res.Body)

	ms := func(t time.Time) string {
		if t.IsZero() {
			return "n/a" // QUIC does not fire the TCP/TLS trace hooks
		}
		return fmt.Sprintf("%.1fms", float64(t.Sub(start).Microseconds())/1000)
	}
	fmt.Printf("asked=%-3s negotiated=%-9s status=%d bytes=%-4d connect=%-7s tls=%-7s ttfb=%-7s  %s\n",
		proto, res.Proto, res.StatusCode, n, ms(connectDone), ms(tlsDone), ms(firstByte), url)
	if res.StatusCode != http.StatusOK || n == 0 {
		return fmt.Errorf("%s: status=%d bytes=%d - edge answered but body is empty; check Caddyfile site matching", proto, res.StatusCode, n)
	}
	return nil
}

type result struct {
	Name        string  `json:"name"`
	Proto       string  `json:"proto"`
	URL         string  `json:"url"`
	Mode        string  `json:"mode"`        // open-loop (fixed arrival rate) or closed-loop
	Conns       int     `json:"connections"` // h2/h3: multiplexed connections; h1: pools
	RateTarget  float64 `json:"rate_target"` // offered load, open-loop only
	Requests    int     `json:"requests"`
	Errors      int64   `json:"errors"`
	MaxInFlight int64   `json:"max_in_flight"` // peak concurrency = pool-pressure analogue
	WallSeconds float64 `json:"wall_seconds"`
	AchievedRPS float64 `json:"achieved_rps"`
	P50ms       float64 `json:"p50_ms"`
	P95ms       float64 `json:"p95_ms"`
	P99ms       float64 `json:"p99_ms"`
	MaxMs       float64 `json:"max_ms"`
}

func load(proto, url string, n, c, conns int, rate float64, dur time.Duration, name, out string) error {
	// The connection model is a controlled variable, not an accident. For
	// h2/h3 each entry is ONE multiplexed connection = one congestion
	// controller: conns=1 is the lecture's "one connection, many streams";
	// conns=N spreads the streams over N congestion windows. For h1 each entry
	// is a pool (no multiplexing exists to constrain).
	if conns < 1 {
		conns = 1
	}
	clients := make([]*http.Client, conns)
	for i := range clients {
		cl, closeFn := clientFor(proto)
		defer closeFn()
		clients[i] = cl
		// Warm up: open the connection / QUIC session before timing.
		if r, err := cl.Get(url); err == nil {
			_, _ = io.Copy(io.Discard, r.Body)
			r.Body.Close()
		}
	}
	var rr int64
	pick := func() *http.Client { return clients[atomic.AddInt64(&rr, 1)%int64(conns)] }

	var (
		mu                    sync.Mutex
		lat                   []float64 // ms
		errs                  int64
		inFlight, maxInFlight int64
	)
	do := func() {
		cur := atomic.AddInt64(&inFlight, 1)
		defer atomic.AddInt64(&inFlight, -1)
		for { // record peak concurrency
			old := atomic.LoadInt64(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt64(&maxInFlight, old, cur) {
				break
			}
		}
		t0 := time.Now()
		res, err := pick().Get(url)
		if err != nil {
			atomic.AddInt64(&errs, 1)
			return
		}
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
		d := float64(time.Since(t0).Microseconds()) / 1000
		mu.Lock()
		lat = append(lat, d)
		mu.Unlock()
	}

	var wg sync.WaitGroup
	start := time.Now()
	mode := "closed-loop"
	if rate > 0 {
		// Open loop, like k6 constant-arrival-rate: requests arrive on a fixed
		// schedule regardless of how fast the server answers, so a slow server
		// cannot throttle the offered load. Only this makes the h1/h2/h3 runs
		// comparable at the "same arrival rate" the task demands.
		mode = "open-loop"
		tick := time.NewTicker(time.Duration(float64(time.Second) / rate))
		deadline := time.After(dur)
	loop:
		for {
			select {
			case <-tick.C:
				wg.Add(1)
				go func() { defer wg.Done(); do() }()
			case <-deadline:
				break loop
			}
		}
		tick.Stop()
		wg.Wait() // let in-flight requests drain; counted in wall time (conservative)
	} else {
		// Closed loop, like h2load -n N -c C: C workers each fire back-to-back.
		perWorker := n / c
		for w := 0; w < c; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					do()
				}
			}()
		}
		wg.Wait()
	}
	wall := time.Since(start).Seconds()

	done := len(lat)
	sort.Float64s(lat)
	pct := func(p float64) float64 {
		if done == 0 {
			return 0
		}
		i := int(float64(done) * p)
		if i >= done {
			i = done - 1
		}
		return lat[i]
	}
	maxMs := 0.0
	if done > 0 {
		maxMs = lat[done-1]
	}
	r := result{
		Name: name, Proto: proto, URL: url, Mode: mode, Conns: conns, RateTarget: rate,
		Requests: done + int(errs), Errors: errs, MaxInFlight: maxInFlight,
		WallSeconds: round(wall, 2), AchievedRPS: round(float64(done)/wall, 1),
		P50ms: round(pct(0.50), 2), P95ms: round(pct(0.95), 2),
		P99ms: round(pct(0.99), 2), MaxMs: round(maxMs, 2),
	}
	b, _ := json.MarshalIndent(r, "", "  ")
	fmt.Printf("RUN %s proto=%s mode=%s conns=%d rate_target=%.0f reqs=%d errs=%d max_inflight=%d wall=%.2fs rps=%.1f  p50=%.2f p95=%.2f p99=%.2f max=%.2f (ms)\n",
		r.Name, r.Proto, r.Mode, r.Conns, r.RateTarget, r.Requests, r.Errors, r.MaxInFlight, r.WallSeconds, r.AchievedRPS, r.P50ms, r.P95ms, r.P99ms, r.MaxMs)
	if out != "" {
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func round(f float64, d int) float64 {
	m := 1.0
	for i := 0; i < d; i++ {
		m *= 10
	}
	return float64(int64(f*m+0.5)) / m
}

func main() {
	mode := flag.String("mode", "prove", "prove | load")
	proto := flag.String("proto", "h3", "h1 | h2 | h3")
	url := flag.String("url", "https://edge:9443/api/quote/1", "target URL")
	n := flag.Int("n", 6000, "total requests (closed-loop load only)")
	c := flag.Int("c", 50, "concurrency (closed-loop load only)")
	rate := flag.Float64("rate", 0, "open-loop arrival rate in req/s; 0 = closed-loop using -n/-c")
	dur := flag.Duration("duration", 60*time.Second, "open-loop duration")
	conns := flag.Int("conns", 1, "independent connections (h2/h3: one multiplexed connection each; h1: pools)")
	name := flag.String("name", "h3", "run name (load mode)")
	out := flag.String("out", "", "write result JSON here (load mode)")
	flag.Parse()

	var err error
	switch *mode {
	case "prove":
		err = prove(*proto, *url)
	case "load":
		err = load(*proto, *url, *n, *c, *conns, *rate, *dur, *name, *out)
	default:
		err = fmt.Errorf("unknown mode %q", *mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
