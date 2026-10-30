package errs

import "connectrpc.com/connect"

var (
	ErrAuthMissingCtx       = &Error{connect.CodeUnauthenticated, "no caller identity", nil}
	ErrTokenInvalid         = &Error{connect.CodeUnauthenticated, "invalid or expired token", nil}
	ErrInvalidCredentials   = &Error{connect.CodeUnauthenticated, "wrong username or password", nil}
	ErrInvalidValue         = &Error{connect.CodeInvalidArgument, "invalid value", nil}
	ErrIdempotencyKeyReused = &Error{connect.CodeInvalidArgument, "idempotency key reused with a different request", nil}
	ErrNotFound             = &Error{connect.CodeNotFound, "not found", nil}
	ErrAlreadyExists        = &Error{connect.CodeAlreadyExists, "already exists", nil}
	ErrPermissionDenied     = &Error{connect.CodePermissionDenied, "permission denied", nil}
	ErrVersionConflict      = &Error{connect.CodeAborted, "version conflict", nil}
	ErrUnavailable          = &Error{connect.CodeUnavailable, "dependency unavailable", nil}
)
