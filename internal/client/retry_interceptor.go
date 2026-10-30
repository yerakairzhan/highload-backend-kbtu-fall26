package client

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
)

// RetryPolicy is the client side of SIS #1's "timeouts and retries": a
// per-attempt deadline, a bounded number of attempts, and exponential backoff
// with full jitter so a burst of failures does not come back as a synchronized
// retry storm.
type RetryPolicy struct {
	MaxAttempts       int
	PerAttemptTimeout time.Duration // 0 = no per-attempt deadline
	BaseBackoff       time.Duration
	MaxBackoff        time.Duration
}

var NoRetry = RetryPolicy{MaxAttempts: 1}

func (p RetryPolicy) backoff(attempt int) time.Duration {
	d := p.BaseBackoff << (attempt - 1)
	if d <= 0 || d > p.MaxBackoff {
		d = p.MaxBackoff
	}
	return time.Duration(rand.Int64N(int64(d) + 1))
}

// RetryStats counts attempts across all calls of one client.
type RetryStats struct {
	Calls, Attempts atomic.Int64
}

type idempotencyKeyed interface {
	GetIdempotencyKey() string
}

// retrySafe decides from the schema, not from a hand-kept list: the proto
// marks reads NO_SIDE_EFFECTS and likes/deletes IDEMPOTENT; a create is safe
// only when it carries an idempotency key.
func (p RetryPolicy) retrySafe(req connect.AnyRequest) bool {
	switch req.Spec().IdempotencyLevel {
	case connect.IdempotencyNoSideEffects, connect.IdempotencyIdempotent:
		return true
	}
	k, ok := req.Any().(idempotencyKeyed)
	return ok && k.GetIdempotencyKey() != ""
}

func retryable(err error, parent context.Context) bool {
	if parent.Err() != nil { // the caller's own deadline is gone: stop
		return false
	}
	switch connect.CodeOf(err) {
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded:
		return true
	}
	var netErr interface{ Timeout() bool }
	return errors.As(err, &netErr) && netErr.Timeout()
}

// RetryInterceptor applies the policy to unary calls. Each attempt gets its
// own deadline, which Connect sends to the server (grpc-timeout or
// Connect-Timeout-Ms), so the server stops working when the client stops
// waiting.
func RetryInterceptor(p RetryPolicy, stats *RetryStats) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			stats.Calls.Add(1)
			maxAttempts := p.MaxAttempts
			if maxAttempts < 1 || !p.retrySafe(req) {
				maxAttempts = 1
			}
			var lastErr error
			for attempt := 1; attempt <= maxAttempts; attempt++ {
				stats.Attempts.Add(1)
				attemptCtx, cancel := ctx, context.CancelFunc(func() {})
				if p.PerAttemptTimeout > 0 {
					attemptCtx, cancel = context.WithTimeout(ctx, p.PerAttemptTimeout)
				}
				res, err := next(attemptCtx, req)
				cancel()
				if err == nil {
					return res, nil
				}
				lastErr = err
				if attempt == maxAttempts || !retryable(err, ctx) {
					break
				}
				select {
				case <-time.After(p.backoff(attempt)):
				case <-ctx.Done():
					return nil, lastErr
				}
			}
			return nil, lastErr
		}
	})
}
