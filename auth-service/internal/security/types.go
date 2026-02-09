package security

import "time"

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash string, password string) bool
}

type JWTSigner interface {
	// SignAccess возвращает access token и его exp.
	SignAccess(userID string, roles []string, sessionID string, now time.Time) (token string, exp time.Time, err error)
}

type RefreshTokens interface {
	// New возвращает raw refresh token (opaque) + token_hash (для БД) + expiresAt
	New(now time.Time) (raw string, hash []byte, exp time.Time, err error)
	// Hash — для Logout/Refresh входящего refresh_token
	Hash(raw string) []byte
}

type Clock interface{ Now() time.Time }
