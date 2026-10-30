package repository

import (
	"context"
	"sync"
	"time"

	"sis2/internal/common/errs"
	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"

	"github.com/samber/mo"
)

type idempotencyKey struct {
	userId uint64
	key    string
}

// IdempotencyKeyRepository stores which entity a create produced, unique per
// (user_id, key). Rows are kept for the lab run; production expires them
// after 24 h.
type IdempotencyKeyRepository struct {
	rows map[idempotencyKey]models.IdempotencyKey
}

var (
	idempotencyKeyRepositoryInstance *IdempotencyKeyRepository
	idempotencyKeyRepositoryOnce     sync.Once
)

func NewIdempotencyKeyRepository() *IdempotencyKeyRepository {
	idempotencyKeyRepositoryOnce.Do(func() {
		r := &IdempotencyKeyRepository{rows: make(map[idempotencyKey]models.IdempotencyKey)}
		database.RegisterTable(func() { r.rows = make(map[idempotencyKey]models.IdempotencyKey) })
		idempotencyKeyRepositoryInstance = r
	})
	return idempotencyKeyRepositoryInstance
}

func (r *IdempotencyKeyRepository) Find(ctx context.Context, userId uint64, key string) mo.Result[*models.IdempotencyKey] {
	defer database.ReadLock(ctx)()
	row, ok := r.rows[idempotencyKey{userId, key}]
	if !ok {
		return mo.Err[*models.IdempotencyKey](errs.ErrNotFound)
	}
	return mo.Ok(&row)
}

func (r *IdempotencyKeyRepository) Save(ctx context.Context, row *models.IdempotencyKey) mo.Result[*models.IdempotencyKey] {
	defer database.WriteLock(ctx)()
	row.CreatedAt = time.Now()
	r.rows[idempotencyKey{row.UserId, row.Key}] = *row
	return mo.Ok(row)
}
