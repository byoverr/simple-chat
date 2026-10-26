package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/byoverr/auth-service/internal/app"
	"github.com/byoverr/auth-service/internal/config"
	loggerConstructor "github.com/byoverr/auth-service/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	logger := loggerConstructor.New(loggerConstructor.Config{
		Env:      cfg.App.Env,
		LogLevel: cfg.App.LogLevel,
		Service:  "auth-service",
	})

	application := app.New(logger, cfg)

	// Start gRPC
	go func() {
		application.GRPCServer.MustRun()
	}()

	// Start HTTP
	go func() {
		logger.Info().Str("addr", cfg.HTTP.Addr).Msg("http auth server started")
		if err := application.HTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("http server failed")
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop

	application.GRPCServer.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := application.HTTPServer.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("http server shutdown error")
	}

	if err := application.Storage.Close(); err != nil {
		logger.Error().Err(err).Msg("failed to close database connection")
	}

	logger.Info().Msg("gracefully stopped")
}
