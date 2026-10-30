package security

import (
	"strings"
	"testing"
	"time"

	"sis2/internal/common"
	"sis2/internal/common/config"
	"sis2/internal/common/errs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cfg = &config.JWTConfig{Secret: "s3cret", Issuer: "blog-platform", AccessTTL: 15 * time.Minute}

func TestAccessToken_RoundTrip(t *testing.T) {
	token, _, err := IssueAccessToken(cfg, common.User{Id: 42, Name: "aidana"}, time.Now())
	require.NoError(t, err)
	user, err := ParseAccessToken(cfg, token)
	require.NoError(t, err)
	assert.Equal(t, common.User{Id: 42, Name: "aidana"}, user)
}

func TestAccessToken_Rejected(t *testing.T) {
	expired, _, _ := IssueAccessToken(cfg, common.User{Id: 1}, time.Now().Add(-time.Hour))
	other := *cfg
	other.Secret = "another"
	foreign, _, _ := IssueAccessToken(&other, common.User{Id: 1}, time.Now())
	valid, _, _ := IssueAccessToken(cfg, common.User{Id: 1}, time.Now())
	parts := strings.Split(valid, ".")
	// header {"alg":"none","typ":"JWT"}, same claims, no signature
	unsigned := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." + parts[1] + "."

	for name, token := range map[string]string{
		"expired": expired, "wrong secret": foreign, "alg none": unsigned, "garbage": "x.y.z",
	} {
		_, err := ParseAccessToken(cfg, token)
		assert.ErrorIs(t, err, errs.ErrTokenInvalid, name)
	}
}
