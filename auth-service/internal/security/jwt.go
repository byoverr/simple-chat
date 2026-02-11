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

type JWTSigner interface {
	// SignAccess возвращает access token и его exp.
	SignAccess(userID string, roles []string, sessionID string, now time.Time) (token string, exp time.Time, err error)
	// ParseAccess validates and parses the access token, returning its claims.
	ParseAccess(tokenStr string) (jwt.MapClaims, error)
}

func NewJWTSigner(cfg JWTConfig) JWTSigner {
	return &jwtSigner{cfg: cfg}
}

func (s *jwtSigner) ParseAccess(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.AccessSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token claims")
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
