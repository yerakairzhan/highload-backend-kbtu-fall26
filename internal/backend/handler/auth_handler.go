package handler

import (
	"context"

	"sis2/internal/backend/service"
	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/pb/pbconnect"
)

// AuthHandler implements pbconnect.AuthServiceHandler.
type AuthHandler struct {
	pbconnect.UnimplementedAuthServiceHandler
	authService *service.AuthService
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{authService: service.NewAuthService()}
}

// Register creates a user and returns its first tokens
func (h *AuthHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthTokens, error) {
	tokens, err := h.authService.Register(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return tokens, nil
}

// Login exchanges a username and password for tokens
func (h *AuthHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthTokens, error) {
	tokens, err := h.authService.Login(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return tokens, nil
}

// RefreshToken trades a refresh token for a new pair
func (h *AuthHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.AuthTokens, error) {
	tokens, err := h.authService.RefreshToken(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return tokens, nil
}
