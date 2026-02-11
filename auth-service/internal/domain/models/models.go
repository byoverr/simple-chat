package models

import "time"

type User struct {
	ID                string
	Email             string
	DisplayName       string
	PasswordHash      string
	Roles             []string
	DisabledAt        *time.Time
	PasswordChangedAt time.Time
}

type RefreshSession struct {
	ID         string
	UserID     string
	TokenHash  []byte
	CreatedAt  time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *string
	DeviceID   *string
	DeviceName *string
	LastUsedAt *time.Time
}

type ClientInfo struct {
	DeviceID   string
	DeviceName string
	AppVersion string
	Platform   string
	IP         string
	UserAgent  string
}

type UserInfo struct {
	UserID      string
	Email       string
	DisplayName string
	Roles       []string
	SessionID   string
}

type AuthPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	UserID           string
	SessionID        string
	Roles            []string
}
