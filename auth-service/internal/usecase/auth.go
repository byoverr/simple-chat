package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/byoverr/auth-service/internal/domain/dto"
	"github.com/byoverr/auth-service/internal/domain/models"
	"github.com/byoverr/auth-service/internal/security"
	"github.com/byoverr/auth-service/internal/storage"
)

type Config struct {
	PasswordMinLen int

	RefreshReuseDetect bool
}

type Service struct {
	st   storage.Store
	pass security.PasswordHasher
	jwt  security.JWTSigner
	rt   security.RefreshTokens
	clk  security.Clock
	cfg  Config
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

func NewService(st storage.Store, pass security.PasswordHasher, jwt security.JWTSigner, rt security.RefreshTokens, cfg Config) *Service {
	// дефолты
	if cfg.PasswordMinLen == 0 {
		cfg.PasswordMinLen = 8
	}
	return &Service{
		st: st, pass: pass, jwt: jwt, rt: rt,
		clk: realClock{}, cfg: cfg,
	}
}

func (s *Service) Register(ctx context.Context, in dto.RegisterIn) (models.AuthPair, error) {
	if err := s.validateRegister(in); err != nil {
		return models.AuthPair{}, err
	}

	now := s.clk.Now()
	hash, err := s.pass.Hash(in.Password)
	if err != nil {
		return models.AuthPair{}, err
	}

	email := normEmail(in.Email)
	displayName := strings.TrimSpace(in.DisplayName)

	var pair models.AuthPair
	err = s.st.WithinTx(ctx, func(ctx context.Context, tx storage.TxStore) error {
		u, err := tx.Users().Create(ctx, storage.CreateUser{
			Email:             email,
			DisplayName:       displayName,
			PasswordHash:      hash,
			Roles:             []string{"user"},
			PasswordChangedAt: now,
		})
		if err != nil {
			return err
		}

		pair2, err := s.issueSessionAndTokens(ctx, tx, u, in.Client, now)
		if err != nil {
			return err
		}
		pair = pair2
		return nil
	})
	if err != nil {
		if errors.Is(err, storage.ErrAlreadyExists) {
			return models.AuthPair{}, ErrAlreadyExists
		}
		return models.AuthPair{}, err
	}
	return pair, nil
}

func (s *Service) Login(ctx context.Context, in dto.LoginIn) (models.AuthPair, error) {
	if err := s.validateLogin(in); err != nil {
		return models.AuthPair{}, err
	}

	now := s.clk.Now()
	email := normEmail(in.Email)

	u, err := s.st.Users().GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return models.AuthPair{}, ErrInvalidCreds
		}
		return models.AuthPair{}, err
	}
	if u.DisabledAt != nil {
		return models.AuthPair{}, ErrUserDisabled
	}
	if !s.pass.Compare(u.PasswordHash, in.Password) {
		return models.AuthPair{}, ErrInvalidCreds
	}

	var pair models.AuthPair
	err = s.st.WithinTx(ctx, func(ctx context.Context, tx storage.TxStore) error {
		pair2, err := s.issueSessionAndTokens(ctx, tx, u, in.Client, now)
		if err != nil {
			return err
		}
		pair = pair2
		return nil
	})
	if err != nil {
		return models.AuthPair{}, err
	}
	return pair, nil
}

func (s *Service) Refresh(ctx context.Context, in dto.RefreshIn) (models.AuthPair, error) {
	if err := s.validateRefresh(in.RefreshToken); err != nil {
		return models.AuthPair{}, err
	}

	now := s.clk.Now()
	oldHash := s.rt.Hash(in.RefreshToken)

	var pair models.AuthPair
	err := s.st.WithinTx(ctx, func(ctx context.Context, tx storage.TxStore) error {
		old, err := tx.Sessions().GetForUpdateByTokenHash(ctx, oldHash)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return ErrUnauthenticated
			}
			return err
		}

		if old.RevokedAt != nil {
			if s.cfg.RefreshReuseDetect {
				_ = tx.Sessions().RevokeAllForUser(ctx, old.UserID, now)
			}
			return ErrSessionRevoked
		}
		if now.After(old.ExpiresAt) {
			return ErrSessionExpired
		}

		u, err := tx.Users().GetByID(ctx, old.UserID)
		if err != nil {
			return err
		}
		if u.DisabledAt != nil {
			return ErrUserDisabled
		}

		if old.CreatedAt.Before(u.PasswordChangedAt) {
			_ = tx.Sessions().RevokeAllForUser(ctx, u.ID, now)
			return ErrUnauthenticated
		}

		raw, newHash, refreshExp, err := s.rt.New(now)
		if err != nil {
			return err
		}
		newSess, err := tx.Sessions().Create(ctx, storage.CreateSession{
			UserID:     u.ID,
			TokenHash:  newHash,
			ExpiresAt:  refreshExp,
			DeviceID:   ptr(in.Client.DeviceID),
			DeviceName: ptr(in.Client.DeviceName),
			IP:         ptr(in.Client.IP),
			UserAgent:  ptr(in.Client.UserAgent),
		})
		if err != nil {
			return err
		}

		rid := newSess.ID
		if err := tx.Sessions().Revoke(ctx, old.ID, &rid, now); err != nil {
			return err
		}
		_ = tx.Sessions().TouchLastUsed(ctx, newSess.ID, now)

		// 5) new access
		at, atExp, err := s.jwt.SignAccess(u.ID, u.Roles, newSess.ID, now)
		if err != nil {
			return err
		}

		pair = models.AuthPair{
			AccessToken:      at,
			AccessExpiresAt:  atExp,
			RefreshToken:     raw,
			RefreshExpiresAt: refreshExp,
			UserID:           u.ID,
			SessionID:        newSess.ID,
			Roles:            u.Roles,
		}
		return nil
	})

	if err != nil {
		return models.AuthPair{}, err
	}
	return pair, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if err := s.validateRefresh(refreshToken); err != nil {
		return err
	}
	now := s.clk.Now()
	h := s.rt.Hash(refreshToken)

	// идемпотентно
	if err := s.st.Sessions().RevokeByTokenHash(ctx, h, now); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil
		}
		return err
	}
	return nil
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	if err := s.validateUserID(userID); err != nil {
		return err
	}
	now := s.clk.Now()
	return s.st.Sessions().RevokeAllForUser(ctx, userID, now)
}

func (s *Service) WhoAmI(ctx context.Context, userID string) (models.UserInfo, error) {
	if err := s.validateUserID(userID); err != nil {
		return models.UserInfo{}, err
	}
	u, err := s.st.Users().GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return models.UserInfo{}, ErrNotFound
		}
		return models.UserInfo{}, err
	}
	if u.DisabledAt != nil {
		return models.UserInfo{}, ErrUserDisabled
	}
	return models.UserInfo{
		UserID:      u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Roles:       u.Roles,
	}, nil
}

func (s *Service) issueSessionAndTokens(ctx context.Context, tx storage.TxStore, u models.User, c models.ClientInfo, now time.Time) (models.AuthPair, error) {
	raw, tokenHash, refreshExp, err := s.rt.New(now)
	if err != nil {
		return models.AuthPair{}, err
	}
	sess, err := tx.Sessions().Create(ctx, storage.CreateSession{
		UserID:     u.ID,
		TokenHash:  tokenHash,
		ExpiresAt:  refreshExp,
		DeviceID:   ptr(c.DeviceID),
		DeviceName: ptr(c.DeviceName),
		IP:         ptr(c.IP),
		UserAgent:  ptr(c.UserAgent),
	})
	if err != nil {
		return models.AuthPair{}, err
	}

	at, atExp, err := s.jwt.SignAccess(u.ID, u.Roles, sess.ID, now)
	if err != nil {
		return models.AuthPair{}, err
	}

	return models.AuthPair{
		AccessToken:      at,
		AccessExpiresAt:  atExp,
		RefreshToken:     raw,
		RefreshExpiresAt: refreshExp,
		UserID:           u.ID,
		SessionID:        sess.ID,
		Roles:            u.Roles,
	}, nil
}

func (s *Service) validateRegister(in dto.RegisterIn) error {
	if strings.TrimSpace(in.Email) == "" || strings.TrimSpace(in.Password) == "" || strings.TrimSpace(in.DisplayName) == "" {
		return ErrInvalidArgument
	}
	if len(in.Password) < s.cfg.PasswordMinLen {
		return ErrInvalidArgument
	}
	return nil
}

func (s *Service) validateLogin(in dto.LoginIn) error {
	if strings.TrimSpace(in.Email) == "" || strings.TrimSpace(in.Password) == "" {
		return ErrInvalidArgument
	}
	return nil
}

func (s *Service) validateRefresh(token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrInvalidArgument
	}
	return nil
}

func (s *Service) validateUserID(userID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrInvalidArgument
	}
	return nil
}

func normEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func ptr(s string) *string {
	v := strings.TrimSpace(s)
	if v == "" {
		return nil
	}
	return &v
}
