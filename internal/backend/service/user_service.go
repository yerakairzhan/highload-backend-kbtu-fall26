package service

import (
	"context"
	"sync"

	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/repository"
	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"
)

const (
	maxDisplayNameLen = 64
	maxBioLen         = 500
)

type UserService struct{}

var (
	userServiceInstance *UserService
	userServiceOnce     sync.Once
)

func NewUserService() *UserService {
	userServiceOnce.Do(func() {
		userServiceInstance = &UserService{}
	})
	return userServiceInstance
}

func (s *UserService) GetMe(ctx context.Context) (*pb.User, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	return user.ToProto(), nil
}

func (s *UserService) GetUser(ctx context.Context, id uint64) (*pb.User, error) {
	result := repository.User().FindById(ctx, id)
	if result.IsError() {
		return nil, result.Error()
	}
	return result.MustGet().ToProto(), nil
}

// UpdateProfile changes only the fields that are set, so sending the same
// update twice gives the same result. Posts and comments keep the author name
// they were written under (it is copied at write time, see SIS #1).
func (s *UserService) UpdateProfile(ctx context.Context, req *pb.UserUpdate) (*pb.User, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Username != nil {
		if err = validateUsername(req.GetUsername()); err != nil {
			return nil, err
		}
	}
	if len(req.GetDisplayName()) > maxDisplayNameLen || len(req.GetBio()) > maxBioLen {
		return nil, errs.ErrInvalidValue.AddErrf("display name up to %d bytes, bio up to %d", maxDisplayNameLen, maxBioLen)
	}
	var updated *models.User
	err = database.Transactional(ctx, func(ctx context.Context) error {
		if req.Username != nil {
			if result := repository.User().Rename(ctx, user.Id, req.GetUsername()); result.IsError() {
				return result.Error()
			}
		}
		result := repository.User().Update(ctx, user.Id, func(u *models.User) error {
			if req.DisplayName != nil {
				u.DisplayName = req.GetDisplayName()
			}
			if req.Bio != nil {
				u.Bio = req.GetBio()
			}
			return nil
		})
		if result.IsError() {
			return result.Error()
		}
		updated = result.MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated.ToProto(), nil
}
