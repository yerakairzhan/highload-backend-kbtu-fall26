package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"sync"
	"time"

	"sis2/internal/backend/security"
	"sis2/internal/common"
	"sis2/internal/common/config"
	"sis2/internal/common/errs"
	"sis2/internal/pb"
	"sis2/internal/repository"
	"sis2/internal/repository/database"
	"sis2/internal/repository/database/models"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)

const (
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt ignores bytes past 72
)

type AuthService struct {
	dummyHash []byte // compared against when the username does not exist
}

var (
	authServiceInstance *AuthService
	authServiceOnce     sync.Once
)

func NewAuthService() *AuthService {
	authServiceOnce.Do(func() {
		dummy, _ := bcrypt.GenerateFromPassword([]byte("timing-equalizer"), bcrypt.DefaultCost)
		authServiceInstance = &AuthService{dummyHash: dummy}
	})
	return authServiceInstance
}

func validateUsername(username string) error {
	if !usernamePattern.MatchString(username) {
		return errs.ErrInvalidValue.AddErrf("username must be 3-32 characters of a-z, 0-9, _")
	}
	return nil
}

// Register creates a user and logs them in. It is not retried by clients: a
// retry after a lost response would get "already exists", so the client logs
// in instead.
func (s *AuthService) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthTokens, error) {
	if err := validateUsername(req.GetUsername()); err != nil {
		return nil, err
	}
	if n := len(req.GetPassword()); n < minPasswordLen || n > maxPasswordLen {
		return nil, errs.ErrInvalidValue.AddErrf("password must be %d-%d bytes", minPasswordLen, maxPasswordLen)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.GetPassword()), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	result := repository.User().Create(ctx, &models.User{
		Username:     req.GetUsername(),
		DisplayName:  req.GetDisplayName(),
		PasswordHash: hash,
	})
	if result.IsError() {
		return nil, result.Error()
	}
	return s.issueTokens(ctx, result.MustGet(), newFamily())
}

// Login checks the password. An unknown username costs the same bcrypt
// comparison as a wrong password, so response time does not reveal which
// usernames exist.
func (s *AuthService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthTokens, error) {
	found := repository.User().FindByUsername(ctx, req.GetUsername())
	hash := s.dummyHash
	if found.IsOk() {
		hash = found.MustGet().PasswordHash
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(req.GetPassword())) != nil || found.IsError() {
		return nil, errs.ErrInvalidCredentials
	}
	return s.issueTokens(ctx, found.MustGet(), newFamily())
}

// RefreshToken trades a refresh token for a new pair. Every refresh token
// works once: presenting one that was already used means it was copied, so
// the whole login (its token family) is revoked.
func (s *AuthService) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.AuthTokens, error) {
	hash := sha256.Sum256([]byte(req.GetRefreshToken()))
	var user *models.User
	var family [16]byte
	err := database.Transactional(ctx, func(ctx context.Context) error {
		found := repository.RefreshToken().Find(ctx, hash)
		if found.IsError() {
			return errs.ErrTokenInvalid.AddErrf("unknown refresh token")
		}
		token := found.MustGet()
		switch {
		case token.Revoked:
			return errs.ErrTokenInvalid.AddErrf("refresh token revoked")
		case token.Used:
			repository.RefreshToken().RevokeFamily(ctx, token.Family)
			log.WithContext(ctx).Warnf("Refresh token reused for user %d: login revoked", token.UserId)
			return errs.ErrTokenInvalid.AddErrf("refresh token already used: this login is revoked")
		case time.Now().After(token.ExpiresAt):
			return errs.ErrTokenInvalid.AddErrf("refresh token expired")
		}
		repository.RefreshToken().MarkUsed(ctx, hash)
		result := repository.User().FindById(ctx, token.UserId)
		if result.IsError() {
			return errs.ErrTokenInvalid.AddErrf("user no longer exists")
		}
		user, family = result.MustGet(), token.Family
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, user, family)
}

func newFamily() [16]byte {
	var f [16]byte
	_, _ = rand.Read(f[:])
	return f
}

func (s *AuthService) issueTokens(ctx context.Context, user *models.User, family [16]byte) (*pb.AuthTokens, error) {
	cfg := &config.GetBlogPlatformConfig().JWT
	now := time.Now()
	access, accessExpires, err := security.IssueAccessToken(cfg, common.User{Id: user.Id, Name: user.Name()}, now)
	if err != nil {
		return nil, err
	}
	var raw [32]byte
	_, _ = rand.Read(raw[:])
	refresh := base64.RawURLEncoding.EncodeToString(raw[:])
	refreshExpires := now.Add(cfg.RefreshTTL)
	repository.RefreshToken().Save(ctx, &models.RefreshToken{
		Hash: sha256.Sum256([]byte(refresh)), UserId: user.Id, Family: family, ExpiresAt: refreshExpires,
	})
	return &pb.AuthTokens{
		AccessToken:      proto.String(access),
		AccessExpiresAt:  timestamppb.New(accessExpires),
		RefreshToken:     proto.String(refresh),
		RefreshExpiresAt: timestamppb.New(refreshExpires),
		User:             user.ToProto(),
	}, nil
}
