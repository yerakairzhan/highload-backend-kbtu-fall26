package common

import "context"

type ctxKey string

const (
	CtxUserKey ctxKey = "user"

	ConfigDirPrefix = "configs"
)

// User is the caller identity. The API gateway verifies the JWT and forwards
// it as X-User-Id / X-User-Name; services never read it from a request body.
type User struct {
	Id   uint64
	Name string
}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, CtxUserKey, u)
}

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(CtxUserKey).(User)
	return u, ok && u.Id != 0
}
