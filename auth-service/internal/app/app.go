package app

import (
	"net/http"

	grpcapp "github.com/byoverr/auth-service/internal/app/grpc"
	"github.com/byoverr/auth-service/internal/config"
	"github.com/byoverr/auth-service/internal/security"
	"github.com/byoverr/auth-service/internal/storage/postgres"
	httphandler "github.com/byoverr/auth-service/internal/transport/http"
	"github.com/byoverr/auth-service/internal/usecase"
	"github.com/rs/zerolog"
)

type App struct {
	GRPCServer *grpcapp.App
	HTTPServer *http.Server
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
	ucCfg := usecase.UsecaseConfig{
		PasswordMinLen:     cfg.Auth.Pass.MinLen,
		RefreshReuseDetect: cfg.Auth.Refresh.ReuseDetect,
	}

	authService := usecase.NewService(log, storage, hasher, jwtSigner, refreshTokens, ucCfg)

	// 4. gRPC
	grpcApp := grpcapp.New(log, authService, jwtSigner, cfg.GRPC.Addr)

	// 5. HTTP
	h := httphandler.NewHandler(authService, log)
	httpSrv := &http.Server{
		Addr:    cfg.HTTP.Addr,
		Handler: h,
	}

	return &App{
		GRPCServer: grpcApp,
		HTTPServer: httpSrv,
		Storage:    storage,
	}
}
