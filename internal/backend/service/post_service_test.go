package service

import (
	"context"
	"fmt"
	"testing"

	"sis2/internal/common"
	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/repository"
	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// asUser returns a context for user id, creating the user row on first use:
// author names are read from it.
func asUser(t *testing.T, id uint64) context.Context {
	ctx := t.Context()
	repository.User().Create(ctx, &models.User{BasicModel: models.BasicModel{Id: id}, Username: fmt.Sprintf("user_%d", id)})
	return common.WithUser(ctx, common.User{Id: id})
}

func createPost(t *testing.T, ctx context.Context, key string) *pb.Post {
	post, err := NewPostService().CreatePost(ctx, &pb.PostCreate{
		Title: proto.String("Title"), Body: proto.String("Body"), Tags: []string{"go"},
		IdempotencyKey: proto.String(key),
	})
	require.NoError(t, err)
	return post
}

func TestPostService_CreatePost_IdempotencyKeyReplays(t *testing.T) {
	database.ReloadInMemoryDB()
	ctx := asUser(t, 1)

	first := createPost(t, ctx, "k1")
	retry := createPost(t, ctx, "k1")

	assert.Equal(t, first.GetId(), retry.GetId(), "a retry with the same key returns the first post")
	list, err := NewPostService().ListPosts(ctx, &pb.PostListRequest{})
	require.NoError(t, err)
	assert.Len(t, list.GetPosts(), 1)
}

func TestPostService_CreatePost_KeyIsScopedToTheUser(t *testing.T) {
	database.ReloadInMemoryDB()
	a := createPost(t, asUser(t, 1), "same")
	b := createPost(t, asUser(t, 2), "same")
	assert.NotEqual(t, a.GetId(), b.GetId())
}

func TestPostService_CreatePost_KeyReusedWithDifferentBody(t *testing.T) {
	database.ReloadInMemoryDB()
	ctx := asUser(t, 1)
	createPost(t, ctx, "k1")

	_, err := NewPostService().CreatePost(ctx, &pb.PostCreate{
		Title: proto.String("Other"), Body: proto.String("Body"), IdempotencyKey: proto.String("k1"),
	})
	assert.ErrorIs(t, err, errs.ErrIdempotencyKeyReused)
}

func TestPostService_CreatePost_RequiresUser(t *testing.T) {
	database.ReloadInMemoryDB()
	_, err := NewPostService().CreatePost(t.Context(), &pb.PostCreate{Title: proto.String("T"), Body: proto.String("B")})
	assert.ErrorIs(t, err, errs.ErrAuthMissingCtx)
}

func TestPostService_UpdatePost_OptimisticLock(t *testing.T) {
	database.ReloadInMemoryDB()
	ctx := asUser(t, 1)
	post := createPost(t, ctx, "")

	updated, err := NewPostService().UpdatePost(ctx, &pb.PostUpdate{
		Id: post.Id, Title: proto.String("New"), Body: proto.String("B"), Version: proto.Int32(1),
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), updated.GetVersion())

	_, err = NewPostService().UpdatePost(ctx, &pb.PostUpdate{
		Id: post.Id, Title: proto.String("Stale"), Body: proto.String("B"), Version: proto.Int32(1),
	})
	assert.ErrorIs(t, err, errs.ErrVersionConflict, "a save based on version 1 must not overwrite version 2")

	_, err = NewPostService().UpdatePost(asUser(t, 2), &pb.PostUpdate{
		Id: post.Id, Title: proto.String("X"), Body: proto.String("B"), Version: proto.Int32(2),
	})
	assert.ErrorIs(t, err, errs.ErrPermissionDenied)
}

func TestPostService_LikePost_IsIdempotent(t *testing.T) {
	database.ReloadInMemoryDB()
	post := createPost(t, asUser(t, 1), "")
	liker := asUser(t, 7)

	require.NoError(t, NewPostService().LikePost(liker, post.GetId()))
	require.NoError(t, NewPostService().LikePost(liker, post.GetId()))
	got, err := NewPostService().GetPost(liker, post.GetId())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), got.GetLikeCount(), "liking twice leaves one like")

	require.NoError(t, NewPostService().UnlikePost(liker, post.GetId()))
	require.NoError(t, NewPostService().UnlikePost(liker, post.GetId()))
	got, _ = NewPostService().GetPost(liker, post.GetId())
	assert.Equal(t, uint64(0), got.GetLikeCount())
}

func TestPostService_ListPosts_CursorPages(t *testing.T) {
	database.ReloadInMemoryDB()
	ctx := asUser(t, 1)
	for range 5 {
		createPost(t, ctx, "")
	}
	seen := map[uint64]bool{}
	req := &pb.PostListRequest{Limit: proto.Int32(2)}
	for pages := 0; pages < 10; pages++ {
		page, err := NewPostService().ListPosts(ctx, req)
		require.NoError(t, err)
		for _, p := range page.GetPosts() {
			assert.False(t, seen[p.GetId()], "a post appears on one page only")
			seen[p.GetId()] = true
		}
		if page.NextCursor == nil {
			break
		}
		req.Cursor = page.NextCursor
	}
	assert.Len(t, seen, 5)
}

func TestPostService_DeletePost_HidesPost(t *testing.T) {
	database.ReloadInMemoryDB()
	ctx := asUser(t, 1)
	post := createPost(t, ctx, "")
	require.NoError(t, NewPostService().DeletePost(ctx, post.GetId()))
	require.NoError(t, NewPostService().DeletePost(ctx, post.GetId()), "deleting twice is safe to retry")
	_, err := NewPostService().GetPost(ctx, post.GetId())
	assert.ErrorIs(t, err, errs.ErrNotFound)
}

func TestPostService_CreatePost_AuthorNameFromStoredUser(t *testing.T) {
	database.ReloadInMemoryDB()
	ctx := asUser(t, 5)
	post := createPost(t, ctx, "")
	assert.Equal(t, "user_5", post.GetAuthorName())
}
