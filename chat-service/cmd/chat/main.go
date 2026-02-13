package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/byoverr/auth-service/internal/config"
	"github.com/byoverr/chat-service/internal/app"
	loggerConstructor "github.com/byoverr/chat-service/pkg/logger"
)

func main() {

	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	logger := loggerConstructor.New(loggerConstructor.Config{Env: cfg.App.Env, LogLevel: cfg.App.LogLevel, Service: "chat-service"})

	application := app.New(logger, cfg)

	go func() {
		application.GRPCServer.MustRun()
	}()

	// Graceful shutdown

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	<-stop

	application.GRPCServer.Stop()

	if err := application.Storage.Close(); err != nil {
		logger.Error().Err(err).Msg("failed to close database connection")
	}

	logger.Info().Msg("Gracefully stopped")
}
