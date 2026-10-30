package loadgen

import (
	"context"
	"fmt"
	"time"

	"sis2/internal/client"

	log "github.com/sirupsen/logrus"
)

type BenchConfig struct {
	URL      string
	Users    int    // seeded users to log in; requests rotate over their tokens
	Password string // the seeded users' password
	Target   client.Target
	Scenario Scenario
	Rate     int
	Duration time.Duration
	Warmup   time.Duration
	Rep      int
}

type BenchResult struct {
	Target      client.Target `json:"target"`
	Scenario    Scenario      `json:"scenario"`
	Rate        int           `json:"rate"`
	DurationSec float64       `json:"duration_sec"`
	Rep         int           `json:"rep"`

	Requests   int     `json:"requests"`
	Failed     int     `json:"failed"`
	Dropped    int     `json:"dropped"`
	FirstError string  `json:"first_error,omitempty"`
	Latency    Latency `json:"latency_ms"`

	WireSentPerReq     float64 `json:"wire_sent_bytes_per_req"`
	WireReceivedPerReq float64 `json:"wire_received_bytes_per_req"`

	NegotiatedProto string    `json:"negotiated_proto"`
	ContentType     string    `json:"content_type"`
	StartedAt       time.Time `json:"started_at"`
}

var wantProto = map[client.Target][2]string{
	client.TargetGRPC:   {"HTTP/2.0", "application/grpc"},
	client.TargetJSONH1: {"HTTP/1.1", "application/json"},
}

// Bench measures one (target, scenario) pair: a warm-up phase that is thrown
// away, then the measured phase. Wire bytes are deltas over the measured
// phase only.
func Bench(ctx context.Context, cfg BenchConfig) (*BenchResult, error) {
	c := client.New(cfg.URL+"/api/v2", cfg.Target, client.NoRetry)
	setup := client.New(cfg.URL+"/api/v2", client.TargetJSONH1, client.NoRetry)
	postIds, err := fetchPostIds(ctx, setup, 1000)
	if err != nil {
		return nil, err
	}
	tokens, err := loginSeedUsers(ctx, setup, cfg.Users, cfg.Password)
	if err != nil {
		return nil, err
	}
	nonce := fmt.Sprintf("%s-%s-r%d-%d", cfg.Scenario, cfg.Target, cfg.Rep, time.Now().UnixNano())
	call := cfg.Scenario.call(c, postIds, tokens, nonce)

	if err := call(ctx, -1); err != nil { // also fills the protocol probe
		return nil, fmt.Errorf("first %s call failed: %w", cfg.Target, err)
	}
	want := wantProto[cfg.Target]
	if c.Probe.Proto != want[0] || c.Probe.ContentType != want[1] || c.Probe.Encoding != "" {
		return nil, fmt.Errorf("%s negotiated %s %q (encoding %q), want %s %q",
			cfg.Target, c.Probe.Proto, c.Probe.ContentType, c.Probe.Encoding, want[0], want[1])
	}
	if cfg.Warmup > 0 {
		log.Infof("%s %s: warm-up %v at %d RPS", cfg.Target, cfg.Scenario, cfg.Warmup, cfg.Rate)
		runOpenLoop(ctx, cfg.Rate, cfg.Warmup, func(ctx context.Context, i int) error { return call(ctx, -2-i) })
	}

	sent0, recv0 := c.Wire.Sent.Load(), c.Wire.Received.Load()
	started := time.Now()
	log.Infof("%s %s: measuring %v at %d RPS", cfg.Target, cfg.Scenario, cfg.Duration, cfg.Rate)
	run := runOpenLoop(ctx, cfg.Rate, cfg.Duration, call)

	done := float64(max(1, len(run.ok)+run.failed))
	return &BenchResult{
		Target: cfg.Target, Scenario: cfg.Scenario, Rate: cfg.Rate,
		DurationSec: cfg.Duration.Seconds(), Rep: cfg.Rep,
		Requests: run.scheduled, Failed: run.failed, Dropped: run.dropped, FirstError: run.firstErr,
		Latency:            summarize(run.ok),
		WireSentPerReq:     float64(c.Wire.Sent.Load()-sent0) / done,
		WireReceivedPerReq: float64(c.Wire.Received.Load()-recv0) / done,
		NegotiatedProto:    c.Probe.Proto, ContentType: c.Probe.ContentType, StartedAt: started,
	}, nil
}
