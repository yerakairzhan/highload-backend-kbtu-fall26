package repository

import (
	"context"
	"sync"

	"sis2/internal/common/errs"
	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"

	"github.com/samber/mo"
)

type RefreshTokenRepository struct {
	rows map[[32]byte]models.RefreshToken
}

var (
	refreshTokenRepositoryInstance *RefreshTokenRepository
	refreshTokenRepositoryOnce     sync.Once
)

func NewRefreshTokenRepository() *RefreshTokenRepository {
	refreshTokenRepositoryOnce.Do(func() {
		r := &RefreshTokenRepository{rows: make(map[[32]byte]models.RefreshToken)}
		database.RegisterTable(func() { r.rows = make(map[[32]byte]models.RefreshToken) })
		refreshTokenRepositoryInstance = r
	})
	return refreshTokenRepositoryInstance
}

func (r *RefreshTokenRepository) Save(ctx context.Context, t *models.RefreshToken) mo.Result[*models.RefreshToken] {
	defer database.WriteLock(ctx)()
	r.rows[t.Hash] = *t
	return mo.Ok(t)
}

func (r *RefreshTokenRepository) Find(ctx context.Context, hash [32]byte) mo.Result[*models.RefreshToken] {
	defer database.ReadLock(ctx)()
	t, ok := r.rows[hash]
	if !ok {
		return mo.Err[*models.RefreshToken](errs.ErrNotFound)
	}
	return mo.Ok(&t)
}

func (r *RefreshTokenRepository) MarkUsed(ctx context.Context, hash [32]byte) {
	defer database.WriteLock(ctx)()
	if t, ok := r.rows[hash]; ok {
		t.Used = true
		r.rows[hash] = t
	}
}

// RevokeFamily revokes every token of one login.
func (r *RefreshTokenRepository) RevokeFamily(ctx context.Context, family [16]byte) {
	defer database.WriteLock(ctx)()
	for h, t := range r.rows {
		if t.Family == family {
			t.Revoked = true
			r.rows[h] = t
		}
	}
}
