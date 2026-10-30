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

func register(t *testing.T, username string) *pb.AuthTokens {
	tokens, err := NewAuthService().Register(t.Context(), &pb.RegisterRequest{
		Username: proto.String(username), Password: proto.String("correct horse"),
	})
	require.NoError(t, err)
	return tokens
}

func TestAuthService_RegisterThenLogin(t *testing.T) {
	database.ReloadInMemoryDB()
	registered := register(t, "aidana")
	assert.NotEmpty(t, registered.GetAccessToken())
	assert.Equal(t, "aidana", registered.GetUser().GetUsername())

	_, err := NewAuthService().Register(t.Context(), &pb.RegisterRequest{
		Username: proto.String("aidana"), Password: proto.String("another one"),
	})
	assert.ErrorIs(t, err, errs.ErrAlreadyExists)

	loggedIn, err := NewAuthService().Login(t.Context(), &pb.LoginRequest{
		Username: proto.String("aidana"), Password: proto.String("correct horse"),
	})
	require.NoError(t, err)
	assert.Equal(t, registered.GetUser().GetId(), loggedIn.GetUser().GetId())
}

func TestAuthService_LoginRejectsWrongPasswordAndUnknownUser(t *testing.T) {
	database.ReloadInMemoryDB()
	register(t, "aidana")

	_, err := NewAuthService().Login(t.Context(), &pb.LoginRequest{Username: proto.String("aidana"), Password: proto.String("wrong pass")})
	assert.ErrorIs(t, err, errs.ErrInvalidCredentials)
	_, err = NewAuthService().Login(t.Context(), &pb.LoginRequest{Username: proto.String("nobody"), Password: proto.String("correct horse")})
	assert.ErrorIs(t, err, errs.ErrInvalidCredentials, "an unknown user gets the same answer as a wrong password")
}

func TestAuthService_RegisterValidates(t *testing.T) {
	database.ReloadInMemoryDB()
	for _, req := range []*pb.RegisterRequest{
		{Username: proto.String("ab"), Password: proto.String("long enough")},
		{Username: proto.String("Has Spaces"), Password: proto.String("long enough")},
		{Username: proto.String("aidana"), Password: proto.String("short")},
	} {
		_, err := NewAuthService().Register(t.Context(), req)
		assert.ErrorIs(t, err, errs.ErrInvalidValue, req.GetUsername())
	}
}

func TestAuthService_RefreshTokenRotates(t *testing.T) {
	database.ReloadInMemoryDB()
	first := register(t, "aidana")

	second, err := NewAuthService().RefreshToken(t.Context(), &pb.RefreshTokenRequest{RefreshToken: first.RefreshToken})
	require.NoError(t, err)
	assert.NotEqual(t, first.GetRefreshToken(), second.GetRefreshToken())

	// Replaying the first refresh token means it was copied: the login is revoked,
	// so even the legitimately issued second token stops working.
	_, err = NewAuthService().RefreshToken(t.Context(), &pb.RefreshTokenRequest{RefreshToken: first.RefreshToken})
	assert.ErrorIs(t, err, errs.ErrTokenInvalid)
	_, err = NewAuthService().RefreshToken(t.Context(), &pb.RefreshTokenRequest{RefreshToken: second.RefreshToken})
	assert.ErrorIs(t, err, errs.ErrTokenInvalid)

	_, err = NewAuthService().RefreshToken(t.Context(), &pb.RefreshTokenRequest{RefreshToken: proto.String("made-up")})
	assert.ErrorIs(t, err, errs.ErrTokenInvalid)
}
