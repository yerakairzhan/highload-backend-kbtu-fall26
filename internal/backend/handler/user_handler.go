package handler

import (
	"context"

	"sis2/internal/backend/service"
	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/pb/pbconnect"
)

// UserHandler implements pbconnect.UserServiceHandler.
type UserHandler struct {
	pbconnect.UnimplementedUserServiceHandler
	userService *service.UserService
}

func NewUserHandler() *UserHandler {
	return &UserHandler{userService: service.NewUserService()}
}

// GetMe returns the caller's profile
func (h *UserHandler) GetMe(ctx context.Context, _ *pb.EmptyMessageRequest) (*pb.User, error) {
	user, err := h.userService.GetMe(ctx)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return user, nil
}

// GetUser returns a public profile
func (h *UserHandler) GetUser(ctx context.Context, req *pb.IdRequest) (*pb.User, error) {
	user, err := h.userService.GetUser(ctx, req.GetId())
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return user, nil
}

// UpdateProfile changes the caller's username, display name or bio
func (h *UserHandler) UpdateProfile(ctx context.Context, req *pb.UserUpdate) (*pb.User, error) {
	user, err := h.userService.UpdateProfile(ctx, req)
	if err != nil {
		return nil, errs.ConnectErr(err)
	}
	return user, nil
}
