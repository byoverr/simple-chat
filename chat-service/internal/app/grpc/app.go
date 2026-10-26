package grpcapp

import (
	"fmt"
	"net"

	chatv1 "github.com/byoverr/auth-proto/gen/go/chat/v1"
	chatgrpc "github.com/byoverr/chat-service/internal/transport/grpc/chat"
	chatinterceptor "github.com/byoverr/chat-service/internal/transport/grpc/interceptor"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type App struct {
	log        zerolog.Logger
	gRPCServer *grpc.Server
	addr       string
}

func New(log zerolog.Logger, chatService chatgrpc.ChatUsecase, jwtSecret string, addr string) *App {
	gRPCServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			chatinterceptor.UnaryAuthInterceptor(jwtSecret, log),
		),
		grpc.ChainStreamInterceptor(
			chatinterceptor.StreamAuthInterceptor(jwtSecret, log),
		),
	)

	chatv1.RegisterChatServiceServer(gRPCServer, chatgrpc.NewChatServer(chatService, log))
	reflection.Register(gRPCServer)

	return &App{log: log, gRPCServer: gRPCServer, addr: addr}
}

func (a *App) MustRun() {
	if err := a.Run(); err != nil {
		panic(err)
	}
}

func (a *App) Run() error {
	lis, err := net.Listen("tcp", a.addr)
	if err != nil {
		return fmt.Errorf("grpcapp: listen: %w", err)
	}
	a.log.Info().Str("addr", lis.Addr().String()).Msg("grpc chat server started")
	return a.gRPCServer.Serve(lis)
}

func (a *App) Stop() {
	a.log.Info().Msg("stopping grpc chat server")
	a.gRPCServer.GracefulStop()
}

//type App struct {
//	log        zerolog.Logger
//	gRPCServer *grpc.Server
//	port       string
//}
//
//func New(log zerolog.Logger, authService authgrpс., jwt security.JWTSigner, port string) *App {
//	gRPCServer := grpc.NewServer(
//		grpc.UnaryInterceptor(interceptor.NewAuthInterceptor(jwt, log).Unary()),
//	)
//
//	// Register services
//	authv1.RegisterAuthServiceServer(gRPCServer, authgrpc.NewAuthServer(authService, log))
//
//	// Enable reflection (for grpcurl etc.)
//	reflection.Register(gRPCServer)
//
//	return &App{
//		log:        log,
//		gRPCServer: gRPCServer,
//		port:       port,
//	}
//}
//
//func (a *App) MustRun() {
//	if err := a.Run(); err != nil {
//		panic(err)
//	}
//}
//
//func (a *App) Run() error {
//	const op = "grpcapp.Run"
//
//	lis, err := net.Listen("tcp", a.port)
//	if err != nil {
//		return fmt.Errorf("%s: %w", op, err)
//	}
//
//	a.log.Info().Str("addr", lis.Addr().String()).Msg("grpc server started")
//
//	if err := a.gRPCServer.Serve(lis); err != nil {
//		return fmt.Errorf("%s: %w", op, err)
//	}
//
//	return nil
//}
//
//func (a *App) Stop() {
//	const op = "grpcapp.Stop"
//
//	a.log.Info().Str("op", op).Msg("stopping grpc server")
//
//	a.gRPCServer.GracefulStop()
//}
