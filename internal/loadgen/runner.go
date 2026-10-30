package loadgen

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const maxInFlight = 4096

type runResult struct {
	ok        []time.Duration // latencies of successful calls
	failed    int
	dropped   int // could not start: maxInFlight requests already outstanding
	firstErr  string
	scheduled int
}

// runOpenLoop starts rate requests per second for dur, each in its own
// goroutine at its scheduled time, and waits for all of them. Latency is
// measured from the moment the call starts to the decoded response.
func runOpenLoop(ctx context.Context, rate int, dur time.Duration, call func(ctx context.Context, i int) error) runResult {
	total := int(float64(rate) * dur.Seconds())
	interval := time.Second / time.Duration(rate)
	lat := make([]int64, total) // ns; -1 failed, -2 dropped
	sem := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup
	var firstErr atomic.Value

	start := time.Now()
	for i := range total {
		if d := time.Until(start.Add(time.Duration(i) * interval)); d > 0 {
			time.Sleep(d)
		}
		select {
		case sem <- struct{}{}:
		default:
			lat[i] = -2
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			t0 := time.Now()
			if err := call(ctx, i); err != nil {
				lat[i] = -1
				firstErr.CompareAndSwap(nil, err.Error())
				return
			}
			lat[i] = int64(time.Since(t0))
		}(i)
	}
	wg.Wait()

	res := runResult{scheduled: total, ok: make([]time.Duration, 0, total)}
	for _, v := range lat {
		switch v {
		case -1:
			res.failed++
		case -2:
			res.dropped++
		default:
			res.ok = append(res.ok, time.Duration(v))
		}
	}
	if e, ok := firstErr.Load().(string); ok {
		res.firstErr = e
	}
	return res
}
