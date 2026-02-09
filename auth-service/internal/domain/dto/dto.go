package dto

import "github.com/byoverr/auth-service/internal/domain/models"

type RegisterIn struct {
	Email       string
	Password    string
	DisplayName string
	Client      models.ClientInfo
}

type LoginIn struct {
	Email    string
	Password string
	Client   models.ClientInfo
}

type RefreshIn struct {
	RefreshToken string
	Client       models.ClientInfo
}
