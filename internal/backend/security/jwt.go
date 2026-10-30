// Package security issues and verifies access tokens.
package security

import (
	"errors"
	"strconv"
	"time"

	"sis2/internal/common"
	"sis2/internal/common/config"
	"sis2/internal/common/errs"

	"github.com/golang-jwt/jwt/v5"
)

type claims struct {
	Name string `json:"name"`
	jwt.RegisteredClaims
}

// IssueAccessToken signs a short-lived HS256 JWT: sub = user id, name = the
// author name the user writes under.
func IssueAccessToken(cfg *config.JWTConfig, user common.User, now time.Time) (string, time.Time, error) {
	expires := now.Add(cfg.AccessTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Name: user.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(user.Id, 10),
			Issuer:    cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	})
	signed, err := token.SignedString([]byte(cfg.Secret))
	return signed, expires, err
}

// ParseAccessToken verifies signature, algorithm, issuer and expiry. Pinning
// HS256 matters: without it a token could pick its own algorithm ("none").
func ParseAccessToken(cfg *config.JWTConfig, raw string) (common.User, error) {
	var c claims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) {
		return []byte(cfg.Secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(cfg.Issuer), jwt.WithExpirationRequired())
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return common.User{}, errs.ErrTokenInvalid.AddErrf("access token expired")
		}
		return common.User{}, errs.ErrTokenInvalid.AddErrf("%v", err)
	}
	id, err := strconv.ParseUint(c.Subject, 10, 64)
	if err != nil || id == 0 {
		return common.User{}, errs.ErrTokenInvalid.AddErrf("bad subject")
	}
	return common.User{Id: id, Name: c.Name}, nil
}
