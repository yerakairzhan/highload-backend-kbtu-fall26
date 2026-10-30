package service

import (
	"testing"

	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/repository/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestCommentService_CreateComment_IdempotencyKeyReplays(t *testing.T) {
	database.ReloadInMemoryDB()
	post := createPost(t, asUser(t, 1), "")
	ctx := asUser(t, 2)
	req := &pb.CommentCreate{PostId: post.Id, Body: proto.String("hi"), IdempotencyKey: proto.String("c1")}

	first, err := NewCommentService().CreateComment(ctx, req)
	require.NoError(t, err)
	retry, err := NewCommentService().CreateComment(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, first.GetId(), retry.GetId())
	got, _ := NewPostService().GetPost(ctx, post.GetId())
	assert.Equal(t, uint64(1), got.GetCommentCount(), "a replay must not count twice")
}

func TestCommentService_CreateComment_ReplyMustBeOnSamePost(t *testing.T) {
	database.ReloadInMemoryDB()
	ctx := asUser(t, 1)
	postA := createPost(t, ctx, "")
	postB := createPost(t, ctx, "")
	parent, err := NewCommentService().CreateComment(ctx, &pb.CommentCreate{PostId: postA.Id, Body: proto.String("p")})
	require.NoError(t, err)

	_, err = NewCommentService().CreateComment(ctx, &pb.CommentCreate{
		PostId: postB.Id, ParentId: parent.Id, Body: proto.String("reply"),
	})
	assert.ErrorIs(t, err, errs.ErrInvalidValue)

	reply, err := NewCommentService().CreateComment(ctx, &pb.CommentCreate{
		PostId: postA.Id, ParentId: parent.Id, Body: proto.String("reply"),
	})
	require.NoError(t, err)
	assert.Equal(t, parent.GetId(), reply.GetParentId())
}

func TestCommentService_ListComments_OldestFirstWithCursor(t *testing.T) {
	database.ReloadInMemoryDB()
	ctx := asUser(t, 1)
	post := createPost(t, ctx, "")
	var ids []uint64
	for range 3 {
		c, err := NewCommentService().CreateComment(ctx, &pb.CommentCreate{PostId: post.Id, Body: proto.String("c")})
		require.NoError(t, err)
		ids = append(ids, c.GetId())
	}

	page1, err := NewCommentService().ListComments(ctx, &pb.CommentListRequest{PostId: post.Id, Limit: proto.Int32(2)})
	require.NoError(t, err)
	require.Len(t, page1.GetComments(), 2)
	assert.Equal(t, ids[0], page1.GetComments()[0].GetId())

	page2, err := NewCommentService().ListComments(ctx, &pb.CommentListRequest{
		PostId: post.Id, Limit: proto.Int32(2), Cursor: page1.NextCursor,
	})
	require.NoError(t, err)
	require.Len(t, page2.GetComments(), 1)
	assert.Equal(t, ids[2], page2.GetComments()[0].GetId())
	assert.Nil(t, page2.NextCursor)
}

func TestCommentService_DeleteComment_LeavesTombstone(t *testing.T) {
	database.ReloadInMemoryDB()
	postAuthor := asUser(t, 1)
	post := createPost(t, postAuthor, "")
	c, err := NewCommentService().CreateComment(asUser(t, 2), &pb.CommentCreate{PostId: post.Id, Body: proto.String("x")})
	require.NoError(t, err)

	assert.ErrorIs(t, NewCommentService().DeleteComment(asUser(t, 3), c.GetId()), errs.ErrPermissionDenied)
	require.NoError(t, NewCommentService().DeleteComment(postAuthor, c.GetId()), "the post author may delete")

	list, err := NewCommentService().ListComments(postAuthor, &pb.CommentListRequest{PostId: post.Id})
	require.NoError(t, err)
	require.Len(t, list.GetComments(), 1)
	assert.True(t, list.GetComments()[0].GetDeleted())
	assert.Empty(t, list.GetComments()[0].GetBody())
}
