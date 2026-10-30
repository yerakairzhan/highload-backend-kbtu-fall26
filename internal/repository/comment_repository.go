package repository

import (
	"context"
	"sync"

	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"

	"github.com/samber/mo"
)

// CommentRepository keeps a per-post index, the in-memory equivalent of the
// (post_id, id) index from the SIS #1 schema: a comment page never scans other
// posts' comments.
type CommentRepository struct {
	BaseRepository[models.Comment, *models.Comment]
	byPost map[uint64][]uint64
}

var (
	commentRepositoryInstance *CommentRepository
	commentRepositoryOnce     sync.Once
)

func NewCommentRepository() *CommentRepository {
	commentRepositoryOnce.Do(func() {
		r := &CommentRepository{
			BaseRepository: newBaseRepository[models.Comment, *models.Comment](),
			byPost:         make(map[uint64][]uint64),
		}
		database.RegisterTable(func() { r.byPost = make(map[uint64][]uint64) })
		commentRepositoryInstance = r
	})
	return commentRepositoryInstance
}

func (r *CommentRepository) Create(ctx context.Context, model *models.Comment) mo.Result[*models.Comment] {
	defer database.WriteLock(ctx)()
	created := r.insert(model)
	r.byPost[created.PostId] = append(r.byPost[created.PostId], created.Id)
	return mo.Ok(created)
}

// ListByPost returns a post's comments oldest first, after cursor.
func (r *CommentRepository) ListByPost(
	ctx context.Context,
	postId uint64,
	cursor uint64,
	limit int,
) []models.Comment {
	defer database.ReadLock(ctx)()
	return paginate(r.tbl.rows, r.byPost[postId], nil, cursor, limit, false)
}
