package interceptor

import (
	"context"
	"strings"

	"sis2/internal/backend/security"
	"sis2/internal/common"
	"sis2/internal/common/config"
	"sis2/internal/common/errs"

	"connectrpc.com/connect"
)

// AuthInterceptor verifies "Authorization: Bearer <JWT>" and puts the user
// into the context. No header = anonymous: reads work, services reject
// writes. A header that does not verify is rejected outright, so a client
// with an expired token learns to refresh instead of silently becoming
// anonymous.
func AuthInterceptor(cfg *config.JWTConfig) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			header := req.Header().Get("Authorization")
			if header == "" {
				return next(ctx, req)
			}
			raw, ok := strings.CutPrefix(header, "Bearer ")
			if !ok {
				return nil, errs.ConnectErr(errs.ErrTokenInvalid.AddErrf("expected a Bearer token"))
			}
			user, err := security.ParseAccessToken(cfg, raw)
			if err != nil {
				return nil, errs.ConnectErr(err)
			}
			return next(common.WithUser(ctx, user), req)
		}
	})
}
