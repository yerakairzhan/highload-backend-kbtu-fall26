package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"

	"sis2/internal/common"
	"sis2/internal/common/errs"
	"sis2/internal/repository"
	"sis2/internal/repository/database/models"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

func requireUser(ctx context.Context) (common.User, error) {
	user, ok := common.UserFromContext(ctx)
	if !ok {
		return common.User{}, errs.ErrAuthMissingCtx.AddErrf("no caller identity in context")
	}
	return user, nil
}

// currentUser loads the caller's stored record: author names come from it,
// not from the token.
func currentUser(ctx context.Context) (*models.User, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	result := repository.User().FindById(ctx, user.Id)
	if result.IsError() {
		return nil, errs.ErrTokenInvalid.AddErrf("user %d no longer exists", user.Id)
	}
	return result.MustGet(), nil
}

// requestHash fingerprints a create request, so reusing an idempotency key
// for a different payload is rejected instead of silently replayed.
func requestHash(parts ...string) [32]byte {
	h := sha256.New()
	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(p), p)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func parseCursor(cursor string) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	v, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil {
		return 0, errs.ErrInvalidValue.AddErrf("invalid cursor %q", cursor)
	}
	return v, nil
}

func pageLimit(limit int32) int {
	switch {
	case limit <= 0:
		return defaultPageLimit
	case limit > maxPageLimit:
		return maxPageLimit
	}
	return int(limit)
}

func nextCursor(lastId uint64, got, limit int) *string {
	if got < limit {
		return nil
	}
	c := strconv.FormatUint(lastId, 10)
	return &c
}
