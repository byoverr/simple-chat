package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/byoverr/auth-service/internal/domain/models"
	"github.com/byoverr/auth-service/internal/storage"
)

type sessionsRepo struct {
	db    *gorm.DB
	table string
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

type SessionsRepo interface {
	Create(ctx context.Context, s CreateSession) (models.RefreshSession, error)

	GetForUpdateByTokenHash(ctx context.Context, tokenHash []byte) (models.RefreshSession, error)

	Revoke(ctx context.Context, id string, replacedBy *string, now time.Time) error
	TouchLastUsed(ctx context.Context, id string, now time.Time) error

	RevokeAllForUser(ctx context.Context, userID string, now time.Time) error

	RevokeByTokenHash(ctx context.Context, tokenHash []byte, now time.Time) error
}

func (r *sessionsRepo) Create(ctx context.Context, in CreateSession) (models.RefreshSession, error) {
	now := time.Now().UTC()
	id := uuid.NewString()

	row := map[string]any{
		"id":          id,
		"user_id":     in.UserID,
		"token_hash":  in.TokenHash,
		"device_id":   in.DeviceID,
		"device_name": in.DeviceName,
		"created_at":  now,
		"expires_at":  in.ExpiresAt,
		"ip":          in.IP,
		"user_agent":  in.UserAgent,
	}

	if err := r.db.WithContext(ctx).Table(r.table).Create(row).Error; err != nil {
		return models.RefreshSession{}, err
	}

	var out sessionRow
	if err := r.db.WithContext(ctx).Table(r.table).
		Select("id,user_id,token_hash,created_at,expires_at,revoked_at,replaced_by").
		Where("id = ?", id).
		First(&out).Error; err != nil {
		return models.RefreshSession{}, err
	}

	return models.RefreshSession{
		ID:         out.ID,
		UserID:     out.UserID,
		TokenHash:  out.TokenHash,
		CreatedAt:  out.CreatedAt,
		ExpiresAt:  out.ExpiresAt,
		RevokedAt:  out.RevokedAt,
		ReplacedBy: out.ReplacedBy,
	}, nil
}

func (r *sessionsRepo) GetForUpdateByTokenHash(ctx context.Context, tokenHash []byte) (models.RefreshSession, error) {
	var row sessionRow
	err := r.db.WithContext(ctx).Table(r.table).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id,user_id,token_hash,created_at,expires_at,revoked_at,replaced_by").
		Where("token_hash = ?", tokenHash).
		First(&row).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.RefreshSession{}, storage.ErrNotFound
		}
		return models.RefreshSession{}, err
	}

	return models.RefreshSession{
		ID:         row.ID,
		UserID:     row.UserID,
		TokenHash:  row.TokenHash,
		CreatedAt:  row.CreatedAt,
		ExpiresAt:  row.ExpiresAt,
		RevokedAt:  row.RevokedAt,
		ReplacedBy: row.ReplacedBy,
	}, nil
}

func (r *sessionsRepo) Revoke(ctx context.Context, id string, replacedBy *string, now time.Time) error {
	upd := map[string]any{
		"revoked_at": now,
	}
	if replacedBy != nil {
		upd["replaced_by"] = *replacedBy
	} else {
		upd["replaced_by"] = nil
	}

	return r.db.WithContext(ctx).Table(r.table).
		Where("id = ?", id).
		Updates(upd).Error
}

func (r *sessionsRepo) TouchLastUsed(ctx context.Context, id string, now time.Time) error {
	return r.db.WithContext(ctx).Table(r.table).
		Where("id = ?", id).
		Update("last_used_at", now).Error
}

func (r *sessionsRepo) RevokeAllForUser(ctx context.Context, userID string, now time.Time) error {
	return r.db.WithContext(ctx).Table(r.table).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}

func (r *sessionsRepo) RevokeByTokenHash(ctx context.Context, tokenHash []byte, now time.Time) error {
	res := r.db.WithContext(ctx).Table(r.table).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		Update("revoked_at", now)

	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return storage.ErrNotFound
	}
	return nil
}
