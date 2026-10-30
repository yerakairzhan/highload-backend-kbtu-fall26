package repository

import (
	"context"
	"sort"
	"time"

	"sis2/internal/common/errs"
	"sis2/internal/repository/database"

	"github.com/samber/mo"
)

type entity[T any] interface {
	*T
	GetId() uint64
	SetId(uint64)
	Touch(time.Time)
}

type table[T any] struct {
	rows  map[uint64]*T
	order []uint64 // ascending id = creation order (Snowflake)
}

// BaseRepository provides generic CRUD over an in-memory table. Embed it in a
// model-specific repository, as with the GORM-backed one it stands in for.
// Every read returns a copy, so callers never share a row with the store.
type BaseRepository[T any, PT entity[T]] struct {
	tbl *table[T]
}

func newBaseRepository[T any, PT entity[T]]() BaseRepository[T, PT] {
	tbl := &table[T]{rows: make(map[uint64]*T)}
	database.RegisterTable(func() {
		tbl.rows = make(map[uint64]*T)
		tbl.order = nil
	})
	return BaseRepository[T, PT]{tbl: tbl}
}

// insert stores a copy of model; the caller holds the write lock.
func (r *BaseRepository[T, PT]) insert(model *T) *T {
	if PT(model).GetId() == 0 {
		PT(model).SetId(database.NextId())
	}
	PT(model).Touch(time.Now())
	stored := *model
	id := PT(&stored).GetId()
	r.tbl.rows[id] = &stored
	if n := len(r.tbl.order); n == 0 || r.tbl.order[n-1] < id {
		r.tbl.order = append(r.tbl.order, id)
	} else { // only seeds with explicit ids land here
		i := sort.Search(n, func(i int) bool { return r.tbl.order[i] >= id })
		r.tbl.order = append(r.tbl.order, 0)
		copy(r.tbl.order[i+1:], r.tbl.order[i:])
		r.tbl.order[i] = id
	}
	out := stored
	return &out
}

func (r *BaseRepository[T, PT]) Create(ctx context.Context, model *T) mo.Result[*T] {
	defer database.WriteLock(ctx)()
	return mo.Ok(r.insert(model))
}

func (r *BaseRepository[T, PT]) FindById(ctx context.Context, id uint64) mo.Result[*T] {
	defer database.ReadLock(ctx)()
	row, ok := r.tbl.rows[id]
	if !ok {
		return mo.Err[*T](errs.ErrNotFound.AddErrf("id %d", id))
	}
	out := *row
	return mo.Ok(&out)
}

// Update applies fn to a copy of the row and stores it only if fn succeeds.
func (r *BaseRepository[T, PT]) Update(ctx context.Context, id uint64, fn func(*T) error) mo.Result[*T] {
	defer database.WriteLock(ctx)()
	row, ok := r.tbl.rows[id]
	if !ok {
		return mo.Err[*T](errs.ErrNotFound.AddErrf("id %d", id))
	}
	updated := *row
	if err := fn(&updated); err != nil {
		return mo.Err[*T](err)
	}
	PT(&updated).Touch(time.Now())
	*row = updated
	out := updated
	return mo.Ok(&out)
}

// Paginate walks rows in id order, keeps those matching cond, and stops at
// limit. cursor is the last id of the previous page (0 = first page); ids are
// time-ordered, so this is keyset pagination, never OFFSET.
func (r *BaseRepository[T, PT]) Paginate(
	ctx context.Context,
	cond func(*T) bool,
	cursor uint64,
	limit int,
	newestFirst bool,
) []T {
	defer database.ReadLock(ctx)()
	return paginate(r.tbl.rows, r.tbl.order, cond, cursor, limit, newestFirst)
}

func paginate[T any](
	rows map[uint64]*T,
	order []uint64,
	cond func(*T) bool,
	cursor uint64,
	limit int,
	newestFirst bool,
) []T {
	out := make([]T, 0, limit)
	if newestFirst {
		start := len(order) - 1
		if cursor > 0 {
			start = sort.Search(len(order), func(i int) bool { return order[i] >= cursor }) - 1
		}
		for i := start; i >= 0 && len(out) < limit; i-- {
			if row := rows[order[i]]; cond == nil || cond(row) {
				out = append(out, *row)
			}
		}
		return out
	}
	start := sort.Search(len(order), func(i int) bool { return order[i] > cursor })
	for i := start; i < len(order) && len(out) < limit; i++ {
		if row := rows[order[i]]; cond == nil || cond(row) {
			out = append(out, *row)
		}
	}
	return out
}
