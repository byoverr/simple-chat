package authgrpc

import (
	"context"
	"errors"
	"strings"

	authv1 "github.com/byoverr/auth-proto/gen/go/auth/v1"
	"github.com/byoverr/auth-service/internal/domain/dto"
	"github.com/byoverr/auth-service/internal/domain/models"
	"github.com/byoverr/auth-service/internal/usecase"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AuthUsecase interface {
	Register(ctx context.Context, in dto.RegisterIn) (models.AuthPair, error)
	Login(ctx context.Context, in dto.LoginIn) (models.AuthPair, error)
	Refresh(ctx context.Context, in dto.RefreshIn) (models.AuthPair, error)

	Logout(ctx context.Context, refreshToken string) error

	WhoAmI(ctx context.Context, userID string) (models.UserInfo, error)
}

type AuthServer struct {
	authv1.UnimplementedAuthServiceServer
	uc  AuthUsecase
	log zerolog.Logger
}

func NewAuthServer(uc AuthUsecase, l zerolog.Logger) *AuthServer {
	return &AuthServer{uc: uc, log: l}
}

// Register(RegisterRequest) -> AuthPair
func (s *AuthServer) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.AuthPair, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if strings.TrimSpace(req.GetEmail()) == "" || strings.TrimSpace(req.GetPassword()) == "" || strings.TrimSpace(req.GetDisplayName()) == "" {
		return nil, status.Error(codes.InvalidArgument, "email, password, display_name are required")
	}

	out, err := s.uc.Register(ctx, dto.RegisterIn{
		Email:       req.GetEmail(),
		Password:    req.GetPassword(),
		DisplayName: req.GetDisplayName(),
		Client:      clientFromPB(req.GetClient()),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return authPairToPB(out), nil
}

// Login(LoginRequest) -> AuthPair
func (s *AuthServer) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.AuthPair, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if strings.TrimSpace(req.GetEmail()) == "" || strings.TrimSpace(req.GetPassword()) == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	out, err := s.uc.Login(ctx, dto.LoginIn{
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
		Client:   clientFromPB(req.GetClient()),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return authPairToPB(out), nil
}

// Refresh(RefreshRequest) -> AuthPair (rotation)
func (s *AuthServer) Refresh(ctx context.Context, req *authv1.RefreshRequest) (*authv1.AuthPair, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if strings.TrimSpace(req.GetRefreshToken()) == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token is required")
	}

	out, err := s.uc.Refresh(ctx, dto.RefreshIn{
		RefreshToken: req.GetRefreshToken(),
		Client:       clientFromPB(req.GetClient()),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return authPairToPB(out), nil
}

// Logout(LogoutRequest) -> Empty
func (s *AuthServer) Logout(ctx context.Context, req *authv1.LogoutRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if strings.TrimSpace(req.GetRefreshToken()) == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token is required")
	}

	if err := s.uc.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, mapErr(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *AuthServer) WhoAmI(ctx context.Context, _ *emptypb.Empty) (*authv1.WhoAmIResponse, error) {
	// TODO: extract user_id from ctx (set by auth interceptor)
	return nil, status.Error(codes.Unimplemented, "WhoAmI requires auth interceptor to extract user_id from JWT")
}

// Helpers

func clientFromPB(c *authv1.ClientInfo) models.ClientInfo {
	if c == nil {
		return models.ClientInfo{}
	}
	return models.ClientInfo{
		DeviceID:   c.GetDeviceId(),
		DeviceName: c.GetDeviceName(),
		AppVersion: c.GetAppVersion(),
		Platform:   c.GetPlatform(),
		IP:         c.GetIp(),
		UserAgent:  c.GetUserAgent(),
	}
}

func authPairToPB(p models.AuthPair) *authv1.AuthPair {
	return &authv1.AuthPair{
		AccessToken:      p.AccessToken,
		AccessExpiresAt:  timestamppb.New(p.AccessExpiresAt),
		RefreshToken:     p.RefreshToken,
		RefreshExpiresAt: timestamppb.New(p.RefreshExpiresAt),
		UserId:           p.UserID,
		SessionId:        p.SessionID,
		Roles:            p.Roles,
	}
}

// mapErr maps domain errors -> gRPC status codes.
// Если service вернул уже status.Error — оставим как есть.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok {
		return st.Err()
	}

	switch {
	case errors.Is(err, usecase.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, usecase.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, usecase.ErrInvalidCreds), errors.Is(err, usecase.ErrUnauthenticated), errors.Is(err, usecase.ErrSessionExpired), errors.Is(err, usecase.ErrSessionRevoked):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, usecase.ErrUserDisabled):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, usecase.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
