package repository

import (
	"context"
	"sync"
	"time"

	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"

	"github.com/samber/mo"
)

type likeKey struct{ postId, userId uint64 }

// LikeRepository is keyed by (post_id, user_id), the table's primary key in
// SIS #1, so a second like by the same user is a no-op, not a second row.
type LikeRepository struct {
	rows map[likeKey]models.Like
}

var (
	likeRepositoryInstance *LikeRepository
	likeRepositoryOnce     sync.Once
)

func NewLikeRepository() *LikeRepository {
	likeRepositoryOnce.Do(func() {
		r := &LikeRepository{rows: make(map[likeKey]models.Like)}
		database.RegisterTable(func() { r.rows = make(map[likeKey]models.Like) })
		likeRepositoryInstance = r
	})
	return likeRepositoryInstance
}

// Add is INSERT ... ON CONFLICT DO NOTHING: inserted reports whether a row was added.
func (r *LikeRepository) Add(ctx context.Context, postId, userId uint64) mo.Result[bool] {
	defer database.WriteLock(ctx)()
	k := likeKey{postId, userId}
	if _, ok := r.rows[k]; ok {
		return mo.Ok(false)
	}
	r.rows[k] = models.Like{PostId: postId, UserId: userId, CreatedAt: time.Now()}
	return mo.Ok(true)
}

// Remove reports whether a row was deleted.
func (r *LikeRepository) Remove(ctx context.Context, postId, userId uint64) mo.Result[bool] {
	defer database.WriteLock(ctx)()
	k := likeKey{postId, userId}
	if _, ok := r.rows[k]; !ok {
		return mo.Ok(false)
	}
	delete(r.rows, k)
	return mo.Ok(true)
}
