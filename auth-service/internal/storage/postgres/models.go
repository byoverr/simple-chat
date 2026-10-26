package postgres

import (
	"time"

	"github.com/lib/pq"
)

type userRow struct {
	ID                string         `gorm:"column:id;primaryKey;type:uuid"`
	Email             string         `gorm:"column:email"`
	DisplayName       string         `gorm:"column:display_name"`
	PasswordHash      string         `gorm:"column:password_hash"`
	Roles             pq.StringArray `gorm:"column:roles;type:text[]"`
	DisabledAt        *time.Time     `gorm:"column:disabled_at"`
	PasswordChangedAt time.Time      `gorm:"column:password_changed_at"`
	CreatedAt         time.Time      `gorm:"column:created_at"`
	UpdatedAt         time.Time      `gorm:"column:updated_at"`
}

type sessionRow struct {
	ID        string `gorm:"column:id;primaryKey;type:uuid"`
	UserID    string `gorm:"column:user_id;type:uuid"`
	TokenHash []byte `gorm:"column:token_hash"`

	CreatedAt time.Time `gorm:"column:created_at"`
	ExpiresAt time.Time `gorm:"column:expires_at"`

	RevokedAt  *time.Time `gorm:"column:revoked_at"`
	ReplacedBy *string    `gorm:"column:replaced_by"`
}
