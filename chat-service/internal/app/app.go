package app

import (
	"net/http"

	grpcapp "github.com/byoverr/chat-service/internal/app/grpc"
	"github.com/byoverr/chat-service/internal/config"
	"github.com/byoverr/chat-service/internal/storage/postgres"
	httphandler "github.com/byoverr/chat-service/internal/transport/http"
	"github.com/byoverr/chat-service/internal/usecase"
	"github.com/rs/zerolog"
)

type App struct {
	GRPCServer *grpcapp.App
	HTTPServer *http.Server
	Storage    *postgres.Store
}

func New(log zerolog.Logger, cfg *config.Config) *App {
	// 1. Storage
	store, err := postgres.Connect(cfg.DB.PostgresURL)
	if err != nil {
		panic("chat-service: connect db: " + err.Error())
	}

	// 2. SSE broker
	broker := httphandler.NewSSEBroker()

	// 3. Usecase
	svc := usecase.NewService(log, store, broker)

	// 4. gRPC app
	grpcApp := grpcapp.New(log, svc, cfg.Auth.JWTSecret, cfg.GRPC.Addr)

	// 5. HTTP handler
	h := httphandler.NewHandler(svc, broker, cfg.Auth.JWTSecret, log)

	srv := &http.Server{
		Addr:    cfg.HTTP.Addr,
		Handler: h,
	}

	return &App{
		GRPCServer: grpcApp,
		HTTPServer: srv,
		Storage:    store,
	}
}
