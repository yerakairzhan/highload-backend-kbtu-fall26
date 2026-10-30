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

func TestUserService_UpdateProfile(t *testing.T) {
	database.ReloadInMemoryDB()
	asUser(t, 2) // takes username user_2
	ctx := asUser(t, 1)

	updated, err := NewUserService().UpdateProfile(ctx, &pb.UserUpdate{
		DisplayName: proto.String("Aidana"), Bio: proto.String("backend"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Aidana", updated.GetDisplayName())
	assert.Equal(t, "user_1", updated.GetUsername(), "unset fields do not change")

	_, err = NewUserService().UpdateProfile(ctx, &pb.UserUpdate{Username: proto.String("user_2")})
	assert.ErrorIs(t, err, errs.ErrAlreadyExists)

	renamed, err := NewUserService().UpdateProfile(ctx, &pb.UserUpdate{Username: proto.String("aidana")})
	require.NoError(t, err)
	assert.Equal(t, "aidana", renamed.GetUsername())

	me, err := NewUserService().GetMe(ctx)
	require.NoError(t, err)
	assert.Equal(t, "aidana", me.GetUsername())

	post := createPost(t, ctx, "")
	assert.Equal(t, "Aidana", post.GetAuthorName(), "new posts use the display name")
}

func TestUserService_GetMeRequiresUser(t *testing.T) {
	database.ReloadInMemoryDB()
	_, err := NewUserService().GetMe(t.Context())
	assert.ErrorIs(t, err, errs.ErrAuthMissingCtx)
}
