package repository

import (
	"context"
	"sync"

	"sis2/internal/common/errs"
	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"

	"github.com/samber/mo"
)

// UserRepository keeps a unique index on username.
type UserRepository struct {
	BaseRepository[models.User, *models.User]
	byUsername map[string]uint64
}

var (
	userRepositoryInstance *UserRepository
	userRepositoryOnce     sync.Once
)

func NewUserRepository() *UserRepository {
	userRepositoryOnce.Do(func() {
		r := &UserRepository{
			BaseRepository: newBaseRepository[models.User, *models.User](),
			byUsername:     make(map[string]uint64),
		}
		database.RegisterTable(func() { r.byUsername = make(map[string]uint64) })
		userRepositoryInstance = r
	})
	return userRepositoryInstance
}

func (r *UserRepository) Create(ctx context.Context, model *models.User) mo.Result[*models.User] {
	defer database.WriteLock(ctx)()
	if _, taken := r.byUsername[model.Username]; taken {
		return mo.Err[*models.User](errs.ErrAlreadyExists.AddErrf("username %q", model.Username))
	}
	created := r.insert(model)
	r.byUsername[created.Username] = created.Id
	return mo.Ok(created)
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) mo.Result[*models.User] {
	defer database.ReadLock(ctx)()
	id, ok := r.byUsername[username]
	if !ok {
		return mo.Err[*models.User](errs.ErrNotFound.AddErrf("username %q", username))
	}
	out := *r.tbl.rows[id]
	return mo.Ok(&out)
}

// Rename moves the unique index entry; the caller holds a transaction.
func (r *UserRepository) Rename(ctx context.Context, id uint64, username string) mo.Result[*models.User] {
	defer database.WriteLock(ctx)()
	if owner, taken := r.byUsername[username]; taken && owner != id {
		return mo.Err[*models.User](errs.ErrAlreadyExists.AddErrf("username %q", username))
	}
	row, ok := r.tbl.rows[id]
	if !ok {
		return mo.Err[*models.User](errs.ErrNotFound.AddErrf("user %d", id))
	}
	delete(r.byUsername, row.Username)
	row.Username = username
	r.byUsername[username] = id
	out := *row
	return mo.Ok(&out)
}
