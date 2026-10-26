package interceptor

import (
	"context"
	"strings"

	"github.com/byoverr/auth-service/internal/security"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type AuthInterceptor struct {
	jwt security.JWTSigner
	log zerolog.Logger
}

func NewAuthInterceptor(jwt security.JWTSigner, log zerolog.Logger) *AuthInterceptor {
	return &AuthInterceptor{
		jwt: jwt,
		log: log,
	}
}

func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if isPublicEndpoint(info.FullMethod) {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
		}

		values := md["authorization"]
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
		}

		accessToken := strings.TrimPrefix(values[0], "Bearer ")
		if accessToken == "" {
			return nil, status.Error(codes.Unauthenticated, "authorization token is empty")
		}

		claims, err := i.jwt.ParseAccess(accessToken)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "access token is invalid")
		}

		sub, ok := claims["sub"].(string)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid token payload: sub")
		}

		ctx = context.WithValue(ctx, "user_id", sub)

		return handler(ctx, req)
	}
}

func isPublicEndpoint(method string) bool {
	switch method {
	case "/auth.v1.AuthService/Register",
		"/auth.v1.AuthService/Login",
		"/auth.v1.AuthService/Refresh":
		return true
	}
	return false
}
