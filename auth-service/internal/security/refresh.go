package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"
)

type RefreshConfig struct {
	RefreshTTL time.Duration
}

type refreshTokens struct {
	cfg RefreshConfig
}

func NewRefreshTokens(cfg RefreshConfig) RefreshTokens {
	return &refreshTokens{cfg: cfg}
}

func (r *refreshTokens) New(now time.Time) (string, []byte, time.Time, error) {
	// 32 байта рандома
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, time.Time{}, fmt.Errorf("rand read: %w", err)
	}

	// raw token: base64 url encoded
	raw := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(buf)

	// hash
	hash := r.Hash(raw)

	exp := now.Add(r.cfg.RefreshTTL)

	return raw, hash, exp, nil
}

func (r *refreshTokens) Hash(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}
