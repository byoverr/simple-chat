package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/byoverr/auth-service/internal/domain/models"

	"github.com/byoverr/auth-service/internal/storage"
)

type usersRepo struct {
	db    *gorm.DB
	table string
}

func (r *usersRepo) Create(ctx context.Context, in storage.CreateUser) (models.User, error) {
	now := time.Now().UTC()
	id := uuid.NewString()

	row := userRow{
		ID:                id,
		Email:             in.Email,
		DisplayName:       in.DisplayName,
		PasswordHash:      in.PasswordHash,
		Roles:             pq.StringArray(in.Roles),
		PasswordChangedAt: in.PasswordChangedAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	err := r.db.WithContext(ctx).Table(r.table).Create(&row).Error
	if err != nil {
		if isUniqueViolation(err) {
			return models.User{}, storage.ErrAlreadyExists
		}
		return models.User{}, err
	}

	return models.User{
		ID:                id,
		Email:             in.Email,
		DisplayName:       in.DisplayName,
		PasswordHash:      in.PasswordHash,
		Roles:             in.Roles,
		PasswordChangedAt: in.PasswordChangedAt,
	}, nil
}

func (r *usersRepo) GetByEmail(ctx context.Context, email string) (models.User, error) {
	var row userRow
	err := r.db.WithContext(ctx).Table(r.table).
		Select("id,email,display_name,password_hash,roles,disabled_at,password_changed_at").
		Where("email = ?", email).
		First(&row).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.User{}, storage.ErrNotFound
		}
		return models.User{}, err
	}

	return models.User{
		ID:                row.ID,
		Email:             row.Email,
		DisplayName:       row.DisplayName,
		PasswordHash:      row.PasswordHash,
		Roles:             []string(row.Roles),
		DisabledAt:        row.DisabledAt,
		PasswordChangedAt: row.PasswordChangedAt,
	}, nil
}

func (r *usersRepo) GetByID(ctx context.Context, id string) (models.User, error) {
	var row userRow
	err := r.db.WithContext(ctx).Table(r.table).
		Select("id,email,display_name,password_hash,roles,disabled_at,password_changed_at").
		Where("id = ?", id).
		First(&row).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.User{}, storage.ErrNotFound
		}
		return models.User{}, err
	}

	return models.User{
		ID:                row.ID,
		Email:             row.Email,
		DisplayName:       row.DisplayName,
		PasswordHash:      row.PasswordHash,
		Roles:             []string(row.Roles),
		DisabledAt:        row.DisabledAt,
		PasswordChangedAt: row.PasswordChangedAt,
	}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
