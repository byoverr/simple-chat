package security

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTConfig struct {
	AccessSecret string
	AccessTTL    time.Duration
	Issuer       string
}

type jwtSigner struct {
	cfg JWTConfig
}

func NewJWTSigner(cfg JWTConfig) JWTSigner {
	return &jwtSigner{cfg: cfg}
}

func (s *jwtSigner) SignAccess(userID string, roles []string, sessionID string, now time.Time) (string, time.Time, error) {
	exp := now.Add(s.cfg.AccessTTL)
	claims := jwt.MapClaims{
		"sub":   userID,
		"roles": roles,
		"sid":   sessionID,
		"iss":   s.cfg.Issuer,
		"exp":   exp.Unix(),
		"iat":   now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	str, err := token.SignedString([]byte(s.cfg.AccessSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return str, exp, nil
}
