package interceptor

import (
	"context"
	"time"

	"connectrpc.com/connect"
)

// TimeoutInterceptor caps every RPC at max. The caller's own deadline already
// arrives in the context (grpc-timeout / Connect-Timeout-Ms headers); this
// only shortens a missing or longer one, so no request runs forever.
func TimeoutInterceptor(max time.Duration) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > max {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, max)
				defer cancel()
			}
			return next(ctx, req)
		}
	})
}
