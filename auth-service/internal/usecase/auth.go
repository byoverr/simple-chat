package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/byoverr/auth-service/internal/domain/dto"
	"github.com/byoverr/auth-service/internal/domain/models"
	"github.com/byoverr/auth-service/internal/security"
	"github.com/byoverr/auth-service/internal/storage"
	"github.com/byoverr/auth-service/internal/storage/postgres"
	"github.com/rs/zerolog"
)

type UsecaseConfig struct {
	PasswordMinLen int

	RefreshReuseDetect bool
}

type Service struct {
	st   postgres.StoreInterface
	pass security.PasswordHasher
	jwt  security.JWTSigner
	rt   security.RefreshTokens
	clk  Clock
	cfg  UsecaseConfig
	log  zerolog.Logger
}

type Clock interface{ Now() time.Time }
type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

func NewService(log zerolog.Logger, st postgres.StoreInterface, pass security.PasswordHasher, jwt security.JWTSigner, rt security.RefreshTokens, cfg UsecaseConfig) *Service {
	// дефолты
	if cfg.PasswordMinLen == 0 {
		cfg.PasswordMinLen = 8
	}
	return &Service{
		st: st, pass: pass, jwt: jwt, rt: rt,
		clk: realClock{}, cfg: cfg,
		log: log,
	}
}

func (s *Service) Register(ctx context.Context, in dto.RegisterIn) (models.AuthPair, error) {
	const op = "usecase.Register"
	log := s.log.With().Str("op", op).Str("email", in.Email).Logger()

	if err := s.validateRegister(in); err != nil {
		log.Debug().Err(err).Msg("validation failed")
		return models.AuthPair{}, err
	}

	now := s.clk.Now()
	hash, err := s.pass.Hash(in.Password)
	if err != nil {
		log.Error().Err(err).Msg("hash failed")
		return models.AuthPair{}, err
	}

	email := normEmail(in.Email)
	displayName := strings.TrimSpace(in.DisplayName)

	var pair models.AuthPair
	err = s.st.WithinTx(ctx, func(ctx context.Context, tx postgres.TxStoreInterface) error {
		u, err := tx.Users().Create(ctx, postgres.CreateUser{
			Email:             email,
			DisplayName:       displayName,
			PasswordHash:      hash,
			Roles:             []string{"user"},
			PasswordChangedAt: now,
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to create user")
			return err
		}

		pair2, err := s.issueSessionAndTokens(ctx, tx, u, in.Client, now)
		if err != nil {
			log.Error().Err(err).Msg("failed to issue tokens")
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

	log.Info().Str("user_id", pair.UserID).Msg("user registered")
	return pair, nil
}

func (s *Service) Login(ctx context.Context, in dto.LoginIn) (models.AuthPair, error) {
	const op = "usecase.Login"
	log := s.log.With().Str("op", op).Str("email", in.Email).Logger()

	if err := s.validateLogin(in); err != nil {
		log.Debug().Err(err).Msg("validation failed")
		return models.AuthPair{}, err
	}

	now := s.clk.Now()
	email := normEmail(in.Email)

	u, err := s.st.Users().GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			log.Debug().Err(err).Msg("user not found")
			return models.AuthPair{}, ErrInvalidCreds
		}
		log.Error().Err(err).Msg("failed to get user")
		return models.AuthPair{}, err
	}
	if u.DisabledAt != nil {
		log.Warn().Msg("user disabled")
		return models.AuthPair{}, ErrUserDisabled
	}
	if !s.pass.Compare(u.PasswordHash, in.Password) {
		log.Debug().Msg("invalid password")
		return models.AuthPair{}, ErrInvalidCreds
	}

	var pair models.AuthPair
	err = s.st.WithinTx(ctx, func(ctx context.Context, tx postgres.TxStoreInterface) error {
		pair2, err := s.issueSessionAndTokens(ctx, tx, u, in.Client, now)
		if err != nil {
			log.Error().Err(err).Msg("failed to issue tokens")
			return err
		}
		pair = pair2
		return nil
	})
	if err != nil {
		return models.AuthPair{}, err
	}

	log.Info().Str("user_id", pair.UserID).Msg("user logged in")
	return pair, nil
}

func (s *Service) Refresh(ctx context.Context, in dto.RefreshIn) (models.AuthPair, error) {
	const op = "usecase.Refresh"
	log := s.log.With().Str("op", op).Logger()

	if err := s.validateRefresh(in.RefreshToken); err != nil {
		log.Debug().Err(err).Msg("validation failed")
		return models.AuthPair{}, err
	}

	now := s.clk.Now()
	oldHash := s.rt.Hash(in.RefreshToken)

	var pair models.AuthPair
	err := s.st.WithinTx(ctx, func(ctx context.Context, tx postgres.TxStoreInterface) error {
		old, err := tx.Sessions().GetForUpdateByTokenHash(ctx, oldHash)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				log.Debug().Err(err).Msg("session not found")
				return ErrUnauthenticated
			}
			return err
		}

		if old.RevokedAt != nil {
			log.Warn().Str("session_id", old.ID).Msg("session revoked")
			if s.cfg.RefreshReuseDetect {
				_ = tx.Sessions().RevokeAllForUser(ctx, old.UserID, now)
			}
			return ErrSessionRevoked
		}
		if now.After(old.ExpiresAt) {
			log.Debug().Str("session_id", old.ID).Msg("session expired")
			return ErrSessionExpired
		}

		u, err := tx.Users().GetByID(ctx, old.UserID)
		if err != nil {
			log.Error().Err(err).Str("user_id", old.UserID).Msg("failed to get user")
			return err
		}
		if u.DisabledAt != nil {
			log.Warn().Str("user_id", old.UserID).Msg("user disabled")
			return ErrUserDisabled
		}

		if old.CreatedAt.Before(u.PasswordChangedAt) {
			log.Info().Str("user_id", old.UserID).Msg("password changed, revoking all sessions")
			_ = tx.Sessions().RevokeAllForUser(ctx, u.ID, now)
			return ErrUnauthenticated
		}

		raw, newHash, refreshExp, err := s.rt.New(now)
		if err != nil {
			return err
		}
		newSess, err := tx.Sessions().Create(ctx, postgres.CreateSession{
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
		log.Error().Err(err).Msg("refresh transaction failed")
		return models.AuthPair{}, err
	}

	log.Info().Str("user_id", pair.UserID).Msg("session refreshed")
	return pair, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	const op = "usecase.Logout"
	log := s.log.With().Str("op", op).Logger()

	if err := s.validateRefresh(refreshToken); err != nil {
		return err
	}
	now := s.clk.Now()
	h := s.rt.Hash(refreshToken)

	// идемпотентно
	if err := s.st.Sessions().RevokeByTokenHash(ctx, h, now); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			log.Debug().Msg("session not found (idempotent)")
			return nil
		}
		log.Error().Err(err).Msg("failed to revoke session")
		return err
	}

	log.Info().Msg("session logged out")
	return nil
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	const op = "usecase.LogoutAll"
	log := s.log.With().Str("op", op).Str("user_id", userID).Logger()

	if err := s.validateUserID(userID); err != nil {
		return err
	}
	now := s.clk.Now()
	if err := s.st.Sessions().RevokeAllForUser(ctx, userID, now); err != nil {
		log.Error().Err(err).Msg("failed to revoke all sessions")
		return err
	}
	log.Info().Msg("all sessions revoked")
	return nil
}

func (s *Service) WhoAmI(ctx context.Context, userID string) (models.UserInfo, error) {
	const op = "usecase.WhoAmI"
	log := s.log.With().Str("op", op).Str("user_id", userID).Logger()

	if err := s.validateUserID(userID); err != nil {
		log.Debug().Err(err).Msg("validation failed")
		return models.UserInfo{}, err
	}
	u, err := s.st.Users().GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			log.Debug().Msg("user not found")
			return models.UserInfo{}, ErrNotFound
		}
		log.Error().Err(err).Msg("failed to get user")
		return models.UserInfo{}, err
	}
	if u.DisabledAt != nil {
		log.Warn().Msg("user disabled")
		return models.UserInfo{}, ErrUserDisabled
	}

	log.Info().Msg("user info retrieved")
	return models.UserInfo{
		UserID:      u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Roles:       u.Roles,
	}, nil
}

func (s *Service) issueSessionAndTokens(ctx context.Context, tx postgres.TxStoreInterface, u models.User, c models.ClientInfo, now time.Time) (models.AuthPair, error) {
	raw, tokenHash, refreshExp, err := s.rt.New(now)
	if err != nil {
		return models.AuthPair{}, err
	}
	sess, err := tx.Sessions().Create(ctx, postgres.CreateSession{
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

func isValidPassword(s string) error {
next:
	for name, classes := range map[string][]*unicode.RangeTable{
		"upper case": {unicode.Upper, unicode.Title},
		"lower case": {unicode.Lower},
		"numeric":    {unicode.Number, unicode.Digit},
		"special":    {unicode.Space, unicode.Symbol, unicode.Punct, unicode.Mark},
	} {
		for _, r := range s {
			if unicode.IsOneOf(classes, r) {
				continue next
			}
		}
		return fmt.Errorf("password must have at least one %s character", name)
	}
	return nil
}

func isValidEmail(email string) error {
	e := strings.TrimSpace(email)
	if e == "" {
		return fmt.Errorf("email is empty")
	}
	if len(e) > 254 { // RFC-практика
		return fmt.Errorf("email too long")
	}
	// mail.ParseAddress требует формат вида "a@b.com" (без пробелов и мусора)
	_, err := mail.ParseAddress(e)
	if err != nil {
		return fmt.Errorf("email is not valid")
	}

	// Доп. прагматичная проверка: у домена должна быть точка
	at := strings.LastIndexByte(e, '@')
	if at < 1 || at+1 >= len(e) {
		return fmt.Errorf("email is not valid")
	}
	domain := e[at+1:]
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("email is not valid")
	}
	return nil
}

func (s *Service) validateRegister(in dto.RegisterIn) error {
	if strings.TrimSpace(in.Email) == "" || strings.TrimSpace(in.Password) == "" || strings.TrimSpace(in.DisplayName) == "" {
		return ErrInvalidArgument
	}

	if err := isValidPassword(in.Password); err != nil {
		return err
	}

	if err := isValidEmail(in.Email); err != nil {
		return err
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
