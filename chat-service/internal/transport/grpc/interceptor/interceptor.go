package interceptor

import (
	"context"

	"google.golang.org/grpc/codes"
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
