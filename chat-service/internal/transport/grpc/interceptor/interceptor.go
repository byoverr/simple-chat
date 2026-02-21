package interceptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func UserIDFromContext(ctx context.Context) (string, error) {
	userIDVal := ctx.Value("user_id")
	if userIDVal == nil {
		return "", status.Error(codes.Unauthenticated, "user_id not found in context")
	}
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		return "", status.Error(codes.Unauthenticated, "user_id is invalid")
	}
	return userID, nil
}

func parseJWT(tokenStr, secret string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	sub, _ := claims["sub"].(string)
	return sub, nil
}

func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "metadata is not provided")
	}
	vals := md["authorization"]
	if len(vals) == 0 {
		return "", status.Error(codes.Unauthenticated, "authorization token is not provided")
	}
	tok := strings.TrimPrefix(vals[0], "Bearer ")
	if tok == "" {
		return "", status.Error(codes.Unauthenticated, "authorization token is empty")
	}
	return tok, nil
}

// UnaryAuthInterceptor validates JWT and injects user_id into context.
func UnaryAuthInterceptor(jwtSecret string, log zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		tok, err := extractBearerToken(ctx)
		if err != nil {
			return nil, err
		}
		userID, err := parseJWT(tok, jwtSecret)
		if err != nil {
			log.Debug().Err(err).Msg("invalid access token")
			return nil, status.Error(codes.Unauthenticated, "invalid access token")
		}
		ctx = context.WithValue(ctx, "user_id", userID)
		return handler(ctx, req)
	}
}

// StreamAuthInterceptor validates JWT for streaming RPCs.
func StreamAuthInterceptor(jwtSecret string, log zerolog.Logger) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		tok, err := extractBearerToken(ss.Context())
		if err != nil {
			return err
		}
		userID, err := parseJWT(tok, jwtSecret)
		if err != nil {
			log.Debug().Err(err).Msg("invalid access token")
			return status.Error(codes.Unauthenticated, "invalid access token")
		}
		ctx := context.WithValue(ss.Context(), "user_id", userID)
		return handler(srv, &wrappedStream{ss, ctx})
	}
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
