package app

import (
	"github.com/byoverr/auth-service/internal/app/grpc"
	"github.com/byoverr/auth-service/internal/config"
	"github.com/byoverr/auth-service/internal/security"
	"github.com/byoverr/auth-service/internal/storage/postgres"
	"github.com/byoverr/auth-service/internal/usecase"
	"github.com/rs/zerolog"
)

type App struct {
	GRPCServer *grpcapp.App
	Storage    *postgres.Store
}

func New(
	log zerolog.Logger,
	cfg *config.Config,
) *App {

	// 1. Storage
	storage, err := postgres.Connect(cfg.DB.PostgresURL, "") // schema="" for public or default
	if err != nil {
		panic(err)
	}

	// 2. Security
	hasher := security.NewBcryptHasher(cfg.Auth.Pass.BcryptCost)
	jwtSigner := security.NewJWTSigner(security.JWTConfig{
		AccessSecret: cfg.Auth.JWT.Secret,
		AccessTTL:    cfg.Auth.JWT.AccessTTL,
		Issuer:       "auth-service",
	})
	refreshTokens := security.NewRefreshTokens(security.RefreshConfig{
		RefreshTTL: cfg.Auth.Refresh.TTL,
	})

	// 3. Usecase
	ucCfg := usecase.Config{
		PasswordMinLen:     cfg.Auth.Pass.MinLen,
		RefreshReuseDetect: cfg.Auth.Refresh.ReuseDetect,
	}

	authService := usecase.NewService(storage, hasher, jwtSigner, refreshTokens, ucCfg)

	// 4. gRPC
	grpcApp := grpcapp.New(log, authService, cfg.GRPC.Addr)

	return &App{
		GRPCServer: grpcApp,
		Storage:    storage,
	}
}
