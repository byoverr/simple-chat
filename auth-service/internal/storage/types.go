package storage

import (
	"context"
	"time"

	"github.com/byoverr/auth-service/internal/domain/models"
)

type Store interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx TxStore) error) error

	Users() UsersRepo
	Sessions() SessionsRepo
}

type TxStore interface {
	Users() UsersRepo
	Sessions() SessionsRepo
}

type UsersRepo interface {
	Create(ctx context.Context, u CreateUser) (models.User, error)
	GetByEmail(ctx context.Context, email string) (models.User, error)
	GetByID(ctx context.Context, id string) (models.User, error)
}

type CreateUser struct {
	Email             string
	DisplayName       string
	PasswordHash      string
	Roles             []string
	PasswordChangedAt time.Time
}

type SessionsRepo interface {
	Create(ctx context.Context, s CreateSession) (models.RefreshSession, error)

	GetForUpdateByTokenHash(ctx context.Context, tokenHash []byte) (models.RefreshSession, error)

	Revoke(ctx context.Context, id string, replacedBy *string, now time.Time) error
	TouchLastUsed(ctx context.Context, id string, now time.Time) error

	RevokeAllForUser(ctx context.Context, userID string, now time.Time) error

	RevokeByTokenHash(ctx context.Context, tokenHash []byte, now time.Time) error
}

type CreateSession struct {
	UserID     string
	TokenHash  []byte
	ExpiresAt  time.Time
	DeviceID   *string
	DeviceName *string
	IP         *string
	UserAgent  *string
}
