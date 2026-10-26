package grpcapp

import (
	"fmt"
	"net"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	authv1 "github.com/byoverr/auth-proto/gen/go/auth/v1"
	"github.com/byoverr/auth-service/internal/security"
	authgrpc "github.com/byoverr/auth-service/internal/transport/grpc/auth"
	"github.com/byoverr/auth-service/internal/transport/grpc/interceptor"
)

type App struct {
	log        zerolog.Logger
	gRPCServer *grpc.Server
	port       string
}

func New(log zerolog.Logger, authService authgrpc.AuthUsecase, jwt security.JWTSigner, port string) *App {
	gRPCServer := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.NewAuthInterceptor(jwt, log).Unary()),
	)

	// Register services
	authv1.RegisterAuthServiceServer(gRPCServer, authgrpc.NewAuthServer(authService, log))

	// Enable reflection (for grpcurl etc.)
	reflection.Register(gRPCServer)

	return &App{
		log:        log,
		gRPCServer: gRPCServer,
		port:       port,
	}
}

func (a *App) MustRun() {
	if err := a.Run(); err != nil {
		panic(err)
	}
}

func (a *App) Run() error {
	const op = "grpcapp.Run"

	lis, err := net.Listen("tcp", a.port)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	a.log.Info().Str("addr", lis.Addr().String()).Msg("grpc server started")

	if err := a.gRPCServer.Serve(lis); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (a *App) Stop() {
	const op = "grpcapp.Stop"

	a.log.Info().Str("op", op).Msg("stopping grpc server")

	a.gRPCServer.GracefulStop()
}
