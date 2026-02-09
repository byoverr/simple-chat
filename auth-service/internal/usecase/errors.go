package usecase

import "errors"

var (
	// input
	ErrInvalidArgument = errors.New("invalid argument")

	// auth
	ErrAlreadyExists   = errors.New("already exists")
	ErrInvalidCreds    = errors.New("invalid credentials")
	ErrUserDisabled    = errors.New("user disabled")
	ErrUnauthenticated = errors.New("unauthenticated")

	// sessions
	ErrSessionExpired = errors.New("session expired")
	ErrSessionRevoked = errors.New("session revoked")

	// common
	ErrNotFound         = errors.New("not found")
	ErrPermissionDenied = errors.New("permission denied")
)
