package errs

import (
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
)

// Error carries the Connect code the transport reports: an HTTP status for
// JSON, a grpc-status for gRPC.
type Error struct {
	connectCode connect.Code
	message     string
	details     []string
}

func (e *Error) Error() string {
	if len(e.details) == 0 {
		return e.message
	}
	return fmt.Sprintf("%s: %s", e.message, strings.Join(e.details, "; "))
}

// AddErrf returns a copy with an extra detail; the sentinel is never mutated.
func (e Error) AddErrf(format string, args ...any) *Error {
	cp := make([]string, len(e.details), len(e.details)+1)
	copy(cp, e.details)
	e.details = append(cp, fmt.Sprintf(format, args...))
	return &e
}

// Is matches on the sentinel, so errors.Is keeps working across AddErrf copies.
func (e *Error) Is(target error) bool {
	var t *Error
	return errors.As(target, &t) && t.connectCode == e.connectCode && t.message == e.message
}

func ConnectErr(err error) *connect.Error {
	if err == nil {
		return nil
	}
	var e *Error
	if !errors.As(err, &e) {
		return connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewError(e.connectCode, e)
}
